package jobs

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"kbo-review/internal/google"
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
	s, err := New(st, google.New(""), n)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDuplicateAndPersistentJob(t *testing.T) {
	st := store.NewMemory()
	s := newService(t, st, nil)
	rows, _ := parse.File(strings.NewReader("number,name\n0123456749,A\n0123456749,B"), ".csv")
	created, err := s.Create("x.csv", rows, "", false)
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
	if _, err := s.Create("x.csv", nil, "", false); err != ErrBusy {
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
