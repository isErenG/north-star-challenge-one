// Package jobs owns the in-memory job registry, the processing pipeline and
// the locking around both. Handlers talk to Service; Service never sees HTTP.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kbo-review/internal/dedupe"
	"kbo-review/internal/enrich"
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
	ErrEmptyUpdate  = &Error{400, "Send exactly one of: record edit, email update, accept, verify."}
	ErrNoSuggestion = &Error{404, "No pending suggestion for that field."}
	ErrNotAccepted  = &Error{404, "Nothing to undo for that field."}
	ErrQueued       = &Error{409, "A verification is already queued for this record."}
)

// DefaultConcurrency is the number of records verified at once per job.
const DefaultConcurrency = 4

// RecordEdit is the set of fields a reviewer may change.
type RecordEdit struct {
	ID, Name, Address, Phone, Email, Website, Notes string
	Reviewed                                        bool
}

// Accept applies one suggestion to a record.
type Accept struct {
	ID    string `json:"id"`
	Field string `json:"field"`
}

// Verify re-runs verification for one record.
type Verify struct {
	ID string `json:"id"`
}

// Update carries exactly one operation.
type Update struct {
	Email  *string     `json:"email"`
	Record *RecordEdit `json:"record"`
	Accept *Accept     `json:"accept"`
	Undo   *Accept     `json:"undo"`
	Verify *Verify     `json:"verify"`
}

// CreateOptions configure enrichment for a new job.
type CreateOptions struct {
	Enrich    bool
	BudgetEUR float64
	Sources   []string
}

// Service coordinates jobs. All access to a Job goes through its mutex.
type Service struct {
	mu       sync.Mutex
	jobs     map[string]*model.Job
	budgets  map[string]*enrich.Budget
	store    store.Store
	slots    chan struct{}
	pipeline *enrich.Pipeline
	notify   *notify.Client
	wg       sync.WaitGroup
	defaults CreateOptions
}

// New loads saved jobs and recovers any that were interrupted mid-flight: a
// processing job is marked failed (not silently resumed) and a notification
// caught mid-send is marked delivery unknown rather than sent again.
func New(st store.Store, p *enrich.Pipeline, n *notify.Client, defaultBudgetEUR float64) (*Service, error) {
	s := &Service{jobs: map[string]*model.Job{}, budgets: map[string]*enrich.Budget{}, store: st, slots: make(chan struct{}, MaxConcurrent), pipeline: p, notify: n, defaults: CreateOptions{BudgetEUR: defaultBudgetEUR}}
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
		for i := range j.Records {
			if v := j.Records[i].Verification; v != nil && v.Verdict == model.VerdictNotRun {
				v.Reason = "Interrupted by a restart; verify again."
				v.Verdict = model.VerdictSkipped
			}
		}
		s.jobs[j.ID] = j
		s.budgets[j.ID] = budgetFor(j)
	}
	return s, nil
}

// budgetFor rebuilds a job's budget meter from its saved spend.
func budgetFor(j *model.Job) *enrich.Budget {
	if j.Enrichment == nil {
		return enrich.NewBudget(0)
	}
	b := enrich.NewBudget(j.Enrichment.BudgetEUR)
	b.Reserve(j.Enrichment.SpentEUR)
	return b
}

// EmailReady reports whether completion emails can be requested.
func (s *Service) EmailReady() bool { return s.notify.Ready() }

// Sources reports which verification sources are available.
func (s *Service) Sources() map[string]bool { return s.pipeline.Available() }

// GoogleReady is kept for the config endpoint.
func (s *Service) GoogleReady() bool { return s.Sources()["google"] }

// DefaultBudget is the per-job budget suggested to the upload screen.
func (s *Service) DefaultBudget() float64 { return s.defaults.BudgetEUR }

// Create registers a job and starts processing it in the background.
func (s *Service) Create(filename string, records []model.Record, email string, opts CreateOptions) (*model.Job, error) {
	enrichRequested := opts.Enrich
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
		Total: len(records), Records: records, Email: email, Enrich: enrichRequested, Notification: model.NotifyNotRequested,
	}
	if email != "" {
		j.Notification = model.NotifyPending
	}
	if enrichRequested {
		budget := opts.BudgetEUR
		if budget <= 0 {
			budget = s.defaults.BudgetEUR
		}
		j.Enrichment = &model.Enrichment{Sources: s.pipeline.Resolve(opts.Sources), BudgetEUR: budget, Concurrency: DefaultConcurrency}
	}
	s.mu.Lock()
	s.jobs[j.ID] = j
	s.budgets[j.ID] = budgetFor(j)
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
	ops := 0
	for _, set := range []bool{in.Email != nil, in.Record != nil, in.Accept != nil, in.Undo != nil, in.Verify != nil} {
		if set {
			ops++
		}
	}
	if ops != 1 {
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
	if in.Accept != nil {
		if j.State != model.StateDone {
			s.mu.Unlock()
			return nil, ErrNotDone
		}
		if err := acceptSuggestion(j, in.Accept); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	if in.Undo != nil {
		if j.State != model.StateDone {
			s.mu.Unlock()
			return nil, ErrNotDone
		}
		if err := undoSuggestion(j, in.Undo); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	var verifyRow *model.Record
	if in.Verify != nil {
		if j.State != model.StateDone {
			s.mu.Unlock()
			return nil, ErrNotDone
		}
		row := findRecord(j, in.Verify.ID)
		if row == nil {
			s.mu.Unlock()
			return nil, ErrRecord
		}
		if row.Verification != nil && row.Verification.Verdict == model.VerdictNotRun {
			s.mu.Unlock()
			return nil, ErrQueued
		}
		if j.Enrichment == nil {
			j.Enrichment = &model.Enrichment{Sources: s.pipeline.Resolve(nil), BudgetEUR: s.defaults.BudgetEUR, Concurrency: DefaultConcurrency}
			s.budgets[j.ID] = budgetFor(j)
		}
		row.Verification = &model.Verification{Verdict: model.VerdictNotRun, Reason: "queued", Sources: []string{}, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
		verifyRow = row
	}
	if err := s.store.Save(j); err != nil {
		_ = json.Unmarshal(before, j)
		s.mu.Unlock()
		return nil, ErrSave
	}
	out := snapshot(j)
	shouldNotify := j.State == model.StateDone && j.Notification == model.NotifyPending
	if verifyRow != nil {
		rec := *verifyRow
		idx := indexOf(j, rec.ID)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.verifyOne(j, idx, rec, true)
			s.mu.Lock()
			_ = s.store.Save(j)
			s.mu.Unlock()
		}()
	}
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

func findRecord(j *model.Job, id string) *model.Record {
	for i := range j.Records {
		if j.Records[i].ID == id {
			return &j.Records[i]
		}
	}
	return nil
}

func indexOf(j *model.Job, id string) int {
	for i := range j.Records {
		if j.Records[i].ID == id {
			return i
		}
	}
	return -1
}

// setField writes a suggestion value into the record. Registration status and
// activity are never overwritten; those only leave a note.
func setField(row *model.Record, field, value string) {
	switch field {
	case "name":
		row.Name = value
	case "address":
		row.Address = value
	case "phone":
		row.Phone = value
	case "email":
		row.Email = value
	case "website":
		row.Website = value
	}
}

func appendNote(row *model.Record, line string) {
	if strings.TrimSpace(row.Notes) == "" {
		row.Notes = line
	} else {
		row.Notes = strings.TrimRight(row.Notes, "\n") + "\n" + line
	}
}

func sourcesOf(sg model.Suggestion) string {
	var names []string
	seen := map[string]bool{}
	for _, e := range sg.Evidence {
		if !seen[e.Source] {
			seen[e.Source] = true
			names = append(names, e.Source)
		}
	}
	if len(names) == 0 {
		return "verification"
	}
	return strings.Join(names, "+")
}

// acceptSuggestion copies a pending suggestion into the record.
func acceptSuggestion(j *model.Job, a *Accept) error {
	row := findRecord(j, a.ID)
	if row == nil {
		return ErrRecord
	}
	for i := range row.Suggestions {
		sg := &row.Suggestions[i]
		if sg.Field != a.Field || sg.Accepted {
			continue
		}
		setField(row, sg.Field, sg.Value)
		appendNote(row, fmt.Sprintf("%s accepted from %s on %s: %s", sg.Field, sourcesOf(*sg), time.Now().UTC().Format("2006-01-02"), sg.Value))
		sg.Accepted = true
		validate.Record(row)
		return nil
	}
	return ErrNoSuggestion
}

// undoSuggestion restores the value a suggestion replaced, whether it was
// accepted by a person or applied automatically.
func undoSuggestion(j *model.Job, a *Accept) error {
	row := findRecord(j, a.ID)
	if row == nil {
		return ErrRecord
	}
	for i := range row.Suggestions {
		sg := &row.Suggestions[i]
		if sg.Field != a.Field || !sg.Accepted {
			continue
		}
		setField(row, sg.Field, sg.Current)
		appendNote(row, fmt.Sprintf("%s restored to %q on %s", sg.Field, sg.Current, time.Now().UTC().Format("2006-01-02")))
		sg.Accepted = false
		sg.Auto = false
		validate.Record(row)
		return nil
	}
	return ErrNotAccepted
}

// applyAuto writes automatically accepted suggestions into the record.
func applyAuto(row *model.Record) int {
	n := 0
	for _, sg := range row.Suggestions {
		if !sg.Auto {
			continue
		}
		setField(row, sg.Field, sg.Value)
		appendNote(row, fmt.Sprintf("%s auto-verified from %s on %s: %s", sg.Field, sourcesOf(sg), time.Now().UTC().Format("2006-01-02"), sg.Value))
		n++
	}
	if n > 0 {
		validate.Record(row)
	}
	return n
}

// verifyOne runs the pipeline for record idx and stores the outcome. While
// it runs, the record carries a "running" verification whose trace grows
// step by step so the UI can show live progress.
func (s *Service) verifyOne(j *model.Job, idx int, rec model.Record, force bool) {
	s.mu.Lock()
	opts := enrich.Options{Force: force}
	budget := s.budgets[j.ID]
	if j.Enrichment != nil {
		opts.Sources = j.Enrichment.Sources
	}
	checks := model.Step{Stage: "checks", Status: model.StepDone, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	checks.EndedAt = checks.StartedAt
	checks.Note = fmt.Sprintf("%d data-quality issue(s)", len(rec.Issues))
	if idx >= 0 && idx < len(j.Records) {
		j.Records[idx].Verification = &model.Verification{Verdict: model.VerdictRunning, Reason: "Verifying…", Sources: []string{}, Trace: []model.Step{checks}, CheckedAt: checks.StartedAt}
	}
	s.mu.Unlock()
	opts.OnStep = func(step model.Step) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if idx < 0 || idx >= len(j.Records) || j.Records[idx].Verification == nil {
			return
		}
		v := j.Records[idx].Verification
		trace := append([]model.Step{}, v.Trace...)
		if n := len(trace); n > 0 && trace[n-1].Stage == step.Stage && trace[n-1].Status == model.StepRunning {
			trace[n-1] = step
		} else {
			trace = append(trace, step)
		}
		copyV := *v
		copyV.Trace = trace
		j.Records[idx].Verification = &copyV
	}
	res := s.pipeline.Run(context.Background(), rec, opts, budget)
	res.Verification.Trace = append([]model.Step{checks}, res.Verification.Trace...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if idx < 0 || idx >= len(j.Records) {
		return
	}
	row := &j.Records[idx]
	v := res.Verification
	row.Verification = &v
	row.Suggestions = res.Suggestions
	if res.Place != nil {
		row.Google = res.Place
		row.GoogleError = ""
	}
	auto := applyAuto(row)
	// Cost is deliberately kept out of the UI; the log is the ledger.
	log.Printf("verify job=%s record=%s name=%q verdict=%s cost=€%.4f sources=%s suggestions=%d auto=%d", j.ID, row.ID, row.Name, v.Verdict, v.Cost, strings.Join(v.Sources, ","), len(row.Suggestions), auto)
	if j.Enrichment != nil {
		j.Enrichment.SpentEUR = round4(budget.Spent())
		if v.Verdict == model.VerdictSkipped {
			j.Enrichment.Skipped++
		} else {
			j.Enrichment.Verified++
		}
	}
}

func round4(f float64) float64 { return float64(int(f*10000+0.5)) / 10000 }

// process validates every record, optionally verifies each one against
// real-world sources with a small worker pool, flags duplicates across the
// file and saves the result.
func (s *Service) process(j *model.Job) {
	defer func() { <-s.slots }()
	for i := 0; i < j.Total; i++ {
		s.mu.Lock()
		row := j.Records[i]
		j.Phase = "validation"
		s.mu.Unlock()
		validate.Record(&row)
		s.mu.Lock()
		j.Records[i] = row
		if !j.Enrich {
			j.Progress = i + 1
		}
		s.mu.Unlock()
	}
	if j.Enrich {
		s.enrichAll(j)
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
	if j.Enrichment != nil {
		e := *j.Enrichment
		c.Enrichment = &e
	}
	return &c
}

// IsNotFound reports whether err is the missing-job error.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == 404
}

// enrichAll verifies every record with a bounded worker pool. Progress counts
// verified records so the bar reflects the slow phase.
func (s *Service) enrichAll(j *model.Job) {
	s.mu.Lock()
	j.Phase = "enrich"
	workers := DefaultConcurrency
	if j.Enrichment != nil && j.Enrichment.Concurrency > 0 {
		workers = j.Enrichment.Concurrency
	}
	total := len(j.Records)
	s.mu.Unlock()
	type task struct {
		idx int
		rec model.Record
	}
	tasks := make(chan task)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tasks {
				s.verifyOne(j, t.idx, t.rec, false)
				s.mu.Lock()
				j.Progress++
				if j.Progress%10 == 0 {
					_ = s.store.Save(j)
				}
				s.mu.Unlock()
			}
		}()
	}
	for i := 0; i < total; i++ {
		s.mu.Lock()
		rec := j.Records[i]
		s.mu.Unlock()
		tasks <- task{idx: i, rec: rec}
	}
	close(tasks)
	wg.Wait()
	s.mu.Lock()
	if e := j.Enrichment; e != nil {
		log.Printf("verify job=%s done verified=%d skipped=%d spent=€%.4f budget=€%.2f", j.ID, e.Verified, e.Skipped, e.SpentEUR, e.BudgetEUR)
	}
	s.mu.Unlock()
}
