package jobs

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"kbo-review/internal/enrich"
	"kbo-review/internal/model"
	"kbo-review/internal/notify"
	"kbo-review/internal/parse"
	"kbo-review/internal/store"
)

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newService(t *testing.T, st store.Store, n *notify.Client) *Service {
	t.Helper()
	if n == nil {
		n = notify.New("", "", "")
	}
	s, err := New(st, enrich.NewWith(nil), n, 5)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDuplicateAndPersistentJob(t *testing.T) {
	st := store.NewMemory()
	s := newService(t, st, nil)
	rows, _ := parse.File(strings.NewReader("number,name\n0123456749,A\n0123456749,B"), ".csv")
	created, err := s.Create("x.csv", rows, "", CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	j, err := s.Get(created.ID)
	if err != nil || j.State != model.StateDone || j.Progress != 2 {
		t.Fatalf("job failed: %v %+v", err, j)
	}
	for _, r := range j.Records {
		if !strings.Contains(strings.Join(r.Issues, ","), "Duplicate identifier") {
			t.Fatal("duplicate missed")
		}
	}
	edited, err := s.Apply(created.ID, Update{Record: &RecordEdit{ID: "1", Name: "Edited", Reviewed: true}})
	if err != nil || edited.Records[0].Name != "Edited" || !edited.Records[0].Reviewed {
		t.Fatalf("edit failed: %v %+v", err, edited)
	}
	if !strings.Contains(strings.Join(edited.Records[0].Issues, ","), "Duplicate identifier") {
		t.Fatal("editing cleared duplicate flag")
	}
	if st.Jobs[created.ID].Records[0].Name != "Edited" {
		t.Fatal("edit not persisted")
	}
	if _, err := s.Get("missing"); !IsNotFound(err) {
		t.Fatal("expected not found")
	}
}

func TestInterruptedJobsAreRecoveredAsFailed(t *testing.T) {
	st := store.NewMemory()
	_ = st.Save(&model.Job{ID: "a", State: model.StateProcessing})
	_ = st.Save(&model.Job{ID: "b", State: model.StateDone, Notification: model.NotifySending})
	s := newService(t, st, nil)
	a, _ := s.Get("a")
	b, _ := s.Get("b")
	if a.State != model.StateFailed || a.Error == "" || b.Notification != model.NotifyUnknown {
		t.Fatalf("recovery wrong: %+v %+v", a, b)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	s := newService(t, store.NewMemory(), nil)
	for i := 0; i < MaxConcurrent; i++ {
		s.slots <- struct{}{}
	}
	if _, err := s.Create("x.csv", nil, "", CreateOptions{}); err != ErrBusy {
		t.Fatalf("expected busy, got %v", err)
	}
}

func TestNotificationRequestedOnce(t *testing.T) {
	calls := 0
	n := notify.New("test", "test@example.com", "")
	n.HTTP = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Idempotency-Key") != "kbo-job-notify" {
			t.Error("missing idempotency key")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"test"}`))}, nil
	})}
	s := newService(t, store.NewMemory(), n)
	j := &model.Job{ID: "notify", State: model.StateDone, Email: "user@example.com", Notification: model.NotifyPending, Total: 3}
	s.jobs[j.ID] = j
	s.sendNotification(j)
	s.sendNotification(j)
	if calls != 1 || j.Notification != model.NotifySent {
		t.Fatalf("unexpected notification state: %d %s", calls, j.Notification)
	}
	if _, err := s.Apply(j.ID, Update{Email: &j.Email}); err != ErrEmailQueued {
		t.Fatalf("second request should be refused, got %v", err)
	}
}

type stubSource struct {
	calls int
	mu    sync.Mutex
}

func (st *stubSource) Name() string     { return "website" }
func (st *stubSource) Ready() bool      { return true }
func (st *stubSource) CostEUR() float64 { return 0.5 }
func (st *stubSource) Run(_ context.Context, row *enrich.Row) error {
	st.mu.Lock()
	st.calls++
	st.mu.Unlock()
	row.Signals = append(row.Signals, enrich.Signal{Source: "website", Existence: enrich.Alive, Weight: 0.9, Note: "live"})
	row.Findings = append(row.Findings, enrich.Finding{Source: "website", Field: "email", Value: "owner@" + row.Record.Name + ".be", Confidence: 0.8, URL: "https://x.be"})
	return nil
}

func TestEnrichmentAcceptAndVerify(t *testing.T) {
	src := &stubSource{}
	st := store.NewMemory()
	s, err := New(st, enrich.NewWith(nil, src), notify.New("", "", ""), 5)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := parse.File(strings.NewReader("number,name\n0123456749,alpha\n0123456848,beta\n0123456947,gamma"), ".csv")
	created, err := s.Create("x.csv", rows, "", CreateOptions{Enrich: true, BudgetEUR: 1.2, Sources: []string{"website", "openai"}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Enrichment == nil || created.Enrichment.BudgetEUR != 1.2 || len(created.Enrichment.Sources) != 1 {
		t.Fatalf("enrichment options: %+v", created.Enrichment)
	}
	s.Wait()
	j, _ := s.Get(created.ID)
	if j.State != model.StateDone || j.Progress != 3 || j.Enrichment.Verified != 2 || j.Enrichment.Skipped != 1 || j.Enrichment.SpentEUR != 1 {
		t.Fatalf("budget of 1.2 should allow two 0.5 calls: %+v progress=%d", j.Enrichment, j.Progress)
	}
	var verified *model.Record
	for i := range j.Records {
		if j.Records[i].Verification.Verdict == model.VerdictActive {
			verified = &j.Records[i]
			break
		}
	}
	if verified == nil || len(verified.Suggestions) != 1 || verified.Suggestions[0].Kind != model.KindNew {
		t.Fatalf("suggestions: %+v", j.Records)
	}
	out, err := s.Apply(j.ID, Update{Accept: &Accept{ID: verified.ID, Field: "email"}})
	if err != nil {
		t.Fatal(err)
	}
	got := findRecord(out, verified.ID)
	if got.Email != verified.Suggestions[0].Value || !got.Suggestions[0].Accepted || !strings.Contains(got.Notes, "accepted from website") {
		t.Fatalf("accept: %+v", got)
	}
	if _, err := s.Apply(j.ID, Update{Accept: &Accept{ID: verified.ID, Field: "email"}}); err != ErrNoSuggestion {
		t.Fatalf("second accept: %v", err)
	}
	if _, err := s.Apply(j.ID, Update{Accept: &Accept{ID: "1"}, Verify: &Verify{ID: "1"}}); err != ErrEmptyUpdate {
		t.Fatalf("two ops: %v", err)
	}
	// Verify the budget-skipped row: the budget is exhausted, so it stays skipped but is re-evaluated.
	var skipped string
	for _, r := range j.Records {
		if r.Verification.Skipped == "budget" {
			skipped = r.ID
		}
	}
	out, err = s.Apply(j.ID, Update{Verify: &Verify{ID: skipped}})
	if err != nil || findRecord(out, skipped).Verification.Verdict != model.VerdictNotRun || findRecord(out, skipped).Verification.Sources == nil {
		t.Fatalf("verify queue: %v %+v", err, findRecord(out, skipped).Verification)
	}
	if _, err := s.Apply(j.ID, Update{Verify: &Verify{ID: skipped}}); err != ErrQueued {
		t.Fatalf("double verify: %v", err)
	}
	s.Wait()
	j, _ = s.Get(j.ID)
	if v := findRecord(j, skipped).Verification; v.Verdict != model.VerdictSkipped || v.Skipped != "budget" {
		t.Fatalf("verify over budget: %+v", v)
	}
	if st.Jobs[j.ID].Records[0].Verification == nil {
		t.Fatal("verification not persisted")
	}
}

type agreeingSource struct{ name string }

func (a agreeingSource) Name() string     { return a.name }
func (a agreeingSource) Ready() bool      { return true }
func (a agreeingSource) CostEUR() float64 { return 0 }
func (a agreeingSource) Run(_ context.Context, row *enrich.Row) error {
	row.Signals = append(row.Signals, enrich.Signal{Source: a.name, Existence: enrich.Alive, Weight: 0.6, Note: "live"})
	row.Findings = append(row.Findings, enrich.Finding{Source: a.name, Field: "phone", Value: "+32 3 658 12 34", Confidence: 0.8, URL: "https://" + a.name})
	return nil
}

func TestAutoAcceptAndUndo(t *testing.T) {
	s, err := New(store.NewMemory(), enrich.NewWith(nil, agreeingSource{"google"}, agreeingSource{"website"}), notify.New("", "", ""), 5)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := parse.File(strings.NewReader("number,name,phone\n0123456749,alpha,\n"), ".csv")
	created, _ := s.Create("x.csv", rows, "", CreateOptions{Enrich: true, Sources: []string{"google", "website"}})
	s.Wait()
	j, _ := s.Get(created.ID)
	r := j.Records[0]
	if r.Phone != "+32 3 658 12 34" || len(r.Suggestions) != 1 || !r.Suggestions[0].Auto || !strings.Contains(r.Notes, "auto-verified from google+website") {
		t.Fatalf("auto-accept not applied: %+v", r)
	}
	if _, err := s.Apply(j.ID, Update{Accept: &Accept{ID: r.ID, Field: "phone"}}); err != ErrNoSuggestion {
		t.Fatalf("accepting an auto value again: %v", err)
	}
	out, err := s.Apply(j.ID, Update{Undo: &Accept{ID: r.ID, Field: "phone"}})
	if err != nil || out.Records[0].Phone != "" || out.Records[0].Suggestions[0].Accepted || out.Records[0].Suggestions[0].Auto {
		t.Fatalf("undo: %v %+v", err, out.Records[0])
	}
	if _, err := s.Apply(j.ID, Update{Undo: &Accept{ID: r.ID, Field: "phone"}}); err != ErrNotAccepted {
		t.Fatalf("second undo: %v", err)
	}
	if out, err = s.Apply(j.ID, Update{Accept: &Accept{ID: r.ID, Field: "phone"}}); err != nil || out.Records[0].Phone == "" {
		t.Fatalf("re-accept after undo: %v", err)
	}
}
