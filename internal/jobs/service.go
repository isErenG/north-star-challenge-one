// Package jobs owns the in-memory job registry, the processing pipeline and
// the locking around both. Handlers talk to Service; Service never sees HTTP.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kbo-review/internal/dedupe"
	"kbo-review/internal/google"
	"kbo-review/internal/model"
	"kbo-review/internal/notify"
	"kbo-review/internal/store"
	"kbo-review/internal/validate"
)

// MaxConcurrent is the number of files processed at once.
const MaxConcurrent = 2

// Error carries a user-facing message and a suggested HTTP status.
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

var (
	ErrBusy         = &Error{429, "Two files are already processing. Please try again shortly."}
	ErrNotFound     = &Error{404, "This task was not found. Upload your file to start again."}
	ErrEmailInvalid = &Error{400, "Enter a valid email. Notifications must be configured."}
	ErrEmailQueued  = &Error{409, "A completion email has already been queued."}
	ErrNotDone      = &Error{409, "Wait until processing finishes."}
	ErrRecord       = &Error{404, "Record not found."}
	ErrSave         = &Error{500, "Your changes could not be saved. Please try again."}
	ErrEmptyUpdate  = &Error{400, "Send either a record edit or an email update."}
)

// RecordEdit is the set of fields a reviewer may change.
type RecordEdit struct {
	ID, Name, Address, Phone, Email, Website, Notes string
	Reviewed                                        bool
}

// Update is either an email request or a record edit, never both.
type Update struct {
	Email  *string     `json:"email"`
	Record *RecordEdit `json:"record"`
}

// Service coordinates jobs. All access to a Job goes through its mutex.
type Service struct {
	mu     sync.Mutex
	jobs   map[string]*model.Job
	store  store.Store
	slots  chan struct{}
	google *google.Client
	notify *notify.Client
	wg     sync.WaitGroup
}

// New loads saved jobs and recovers any that were interrupted mid-flight: a
// processing job is marked failed (not silently resumed) and a notification
// caught mid-send is marked delivery unknown rather than sent again.
func New(st store.Store, g *google.Client, n *notify.Client) (*Service, error) {
	s := &Service{jobs: map[string]*model.Job{}, store: st, slots: make(chan struct{}, MaxConcurrent), google: g, notify: n}
	saved, err := st.LoadAll()
	if err != nil {
		return nil, err
	}
	for _, j := range saved {
		if j.State == model.StateProcessing {
			j.State = model.StateFailed
			j.Error = "Processing was interrupted. Please upload your file again."
		}
		if j.Notification == model.NotifySending {
			j.Notification = model.NotifyUnknown
		}
		s.jobs[j.ID] = j
	}
	return s, nil
}

// EmailReady reports whether completion emails can be requested.
func (s *Service) EmailReady() bool { return s.notify.Ready() }

// GoogleReady reports whether Google enrichment can be requested.
func (s *Service) GoogleReady() bool { return s.google.Ready() }

// Create registers a job and starts processing it in the background.
func (s *Service) Create(filename string, records []model.Record, email string, enrich bool) (*model.Job, error) {
	select {
	case s.slots <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	id := make([]byte, 24)
	if _, err := rand.Read(id); err != nil {
		<-s.slots
		return nil, &Error{500, "Could not create a job."}
	}
	j := &model.Job{
		ID: hex.EncodeToString(id), Filename: filepath.Base(filename),
		Created: time.Now().UTC().Format(time.RFC3339), State: model.StateProcessing, Phase: "structure",
		Total: len(records), Records: records, Email: email, Enrich: enrich, Notification: model.NotifyNotRequested,
	}
	if email != "" {
		j.Notification = model.NotifyPending
	}
	s.mu.Lock()
	s.jobs[j.ID] = j
	err := s.store.Save(j)
	if err != nil {
		delete(s.jobs, j.ID)
	}
	out := snapshot(j)
	s.mu.Unlock()
	if err != nil {
		<-s.slots
		return nil, &Error{500, "Could not save the upload."}
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.process(j)
	}()
	return out, nil
}

// Get returns a copy of the job that is safe to read while processing runs.
func (s *Service) Get(id string) (*model.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return snapshot(j), nil
}

// Apply handles a review edit or a notification request.
func (s *Service) Apply(id string, in Update) (*model.Job, error) {
	if (in.Email == nil) == (in.Record == nil) {
		return nil, ErrEmptyUpdate
	}
	s.mu.Lock()
	j, ok := s.jobs[id]
	if !ok {
		s.mu.Unlock()
		return nil, &Error{404, "Task not found."}
	}
	before, _ := json.Marshal(j)
	if in.Email != nil {
		email := strings.TrimSpace(*in.Email)
		if !s.notify.Ready() || !validate.Email(email) {
			s.mu.Unlock()
			return nil, ErrEmailInvalid
		}
		if j.Notification == model.NotifySent || j.Notification == model.NotifySending {
			s.mu.Unlock()
			return nil, ErrEmailQueued
		}
		j.Email = email
		j.Notification = model.NotifyPending
	}
	if in.Record != nil {
		if j.State != model.StateDone {
			s.mu.Unlock()
			return nil, ErrNotDone
		}
		if !editRecord(j, in.Record) {
			s.mu.Unlock()
			return nil, ErrRecord
		}
	}
	if err := s.store.Save(j); err != nil {
		_ = json.Unmarshal(before, j)
		s.mu.Unlock()
		return nil, ErrSave
	}
	out := snapshot(j)
	shouldNotify := j.State == model.StateDone && j.Notification == model.NotifyPending
	s.mu.Unlock()
	if shouldNotify {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.sendNotification(j)
		}()
	}
	return out, nil
}

// Wait blocks until background processing and notifications finish. Used by
// tests and graceful shutdown.
func (s *Service) Wait() { s.wg.Wait() }

func editRecord(j *model.Job, x *RecordEdit) bool {
	for i := range j.Records {
		row := &j.Records[i]
		if row.ID != x.ID {
			continue
		}
		row.Name = strings.TrimSpace(x.Name)
		row.Address = strings.TrimSpace(x.Address)
		row.Phone = strings.TrimSpace(x.Phone)
		row.Email = strings.TrimSpace(x.Email)
		row.Website = strings.TrimSpace(x.Website)
		row.Notes = x.Notes
		row.Reviewed = x.Reviewed
		validate.Record(row)
		return true
	}
	return false
}

// process validates every record, optionally looks up a Google candidate,
// flags duplicates across the file and saves the result.
func (s *Service) process(j *model.Job) {
	defer func() { <-s.slots }()
	for i := 0; i < j.Total; i++ {
		s.mu.Lock()
		row := j.Records[i]
		j.Phase = "validation"
		s.mu.Unlock()
		validate.Record(&row)
		if j.Enrich {
			s.mu.Lock()
			j.Phase = "google"
			s.mu.Unlock()
			row.Google, row.GoogleError = s.google.Lookup(row)
		}
		s.mu.Lock()
		j.Records[i] = row
		j.Progress = i + 1
		s.mu.Unlock()
	}
	s.mu.Lock()
	validate.MarkDuplicates(j.Records)
	dedupe.Cluster(j.Records)
	j.Phase = "complete"
	j.State = model.StateDone
	if err := s.store.Save(j); err != nil {
		j.State = model.StateFailed
		j.Error = "Results could not be saved. Please try again."
	}
	shouldNotify := j.State == model.StateDone && j.Email != ""
	s.mu.Unlock()
	if shouldNotify {
		s.sendNotification(j)
	}
}

// sendNotification sends at most one completion email per job. The pending →
// sending transition is persisted first so a crash mid-send is later reported
// as delivery unknown rather than retried.
func (s *Service) sendNotification(j *model.Job) {
	s.mu.Lock()
	if j.Notification != model.NotifyPending {
		s.mu.Unlock()
		return
	}
	j.Notification = model.NotifySending
	email, id, total := j.Email, j.ID, j.Total
	if s.store.Save(j) != nil {
		j.Notification = model.NotifyFailed
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	status := model.NotifyFailed
	if s.notify.Send(email, id, total) {
		status = model.NotifySent
	}
	s.mu.Lock()
	j.Notification = status
	_ = s.store.Save(j)
	s.mu.Unlock()
}

// snapshot copies a job so callers can encode it after the lock is released.
// Record structs are copied by value; their maps and geometry are read-only
// after parsing.
func snapshot(j *model.Job) *model.Job {
	c := *j
	c.Records = slices.Clone(j.Records)
	return &c
}

// IsNotFound reports whether err is the missing-job error.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == 404
}
