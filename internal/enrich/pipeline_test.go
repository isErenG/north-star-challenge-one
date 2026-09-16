package enrich

import (
	"context"
	"errors"
	"strings"
	"testing"

	"kbo-review/internal/model"
)

type fakeSource struct {
	name     string
	cost     float64
	ready    bool
	signals  []Signal
	findings []Finding
	pages    []Page
	err      error
	calls    int
}

func (f *fakeSource) Name() string     { return f.name }
func (f *fakeSource) Ready() bool      { return f.ready }
func (f *fakeSource) CostEUR() float64 { return f.cost }
func (f *fakeSource) Run(_ context.Context, row *Row) error {
	f.calls++
	row.Signals = append(row.Signals, f.signals...)
	row.Findings = append(row.Findings, f.findings...)
	row.Pages = append(row.Pages, f.pages...)
	return f.err
}

type fakeJudge struct {
	verdict  Signal
	findings []Finding
	verdicts int
	extracts int
}

func (j *fakeJudge) Ready() bool      { return true }
func (j *fakeJudge) CostEUR() float64 { return 0.002 }
func (j *fakeJudge) Verdict(context.Context, *Row) (Signal, error) {
	j.verdicts++
	return j.verdict, nil
}
func (j *fakeJudge) Extract(context.Context, *Row, []string) ([]Finding, error) {
	j.extracts++
	return j.findings, nil
}

func TestPipelineCeasedStopsEarly(t *testing.T) {
	g := &fakeSource{name: "google", cost: 0.03, ready: true, signals: []Signal{{Source: "google", Existence: Closed, Weight: 0.9, Note: "permanently closed"}}, findings: []Finding{{Source: "google", Field: "phone", Value: "03 1", Confidence: 0.7}}}
	w := &fakeSource{name: "website", ready: true, pages: []Page{{Text: "x"}}}
	j := &fakeJudge{}
	p := NewWith(j, g, w)
	rec := model.Record{Name: "X", Status: "Normale toestand"}
	res := p.Run(context.Background(), rec, Options{Sources: []string{"google", "website", "openai"}}, NewBudget(1))
	if res.Verification.Verdict != model.VerdictCeased || j.verdicts != 0 || j.extracts != 0 {
		t.Fatalf("ceased handling: %+v judge=%+v", res.Verification, j)
	}
	if res.Suggestions[0].Field != "status" || res.Suggestions[0].Value != "Likely ceased" || res.Suggestions[0].Current != "Normale toestand" {
		t.Fatalf("status suggestion: %+v", res.Suggestions)
	}
	if res.Verification.Cost != 0.03 || len(res.Verification.Sources) != 2 {
		t.Fatalf("accounting: %+v", res.Verification)
	}
}

func TestPipelineUnclearConsultsJudgeAndFillsGaps(t *testing.T) {
	w := &fakeSource{name: "website", ready: true, signals: []Signal{{Source: "website", Existence: Closed, Weight: 0.4, Note: "no response"}}, pages: []Page{{Text: "x"}}}
	j := &fakeJudge{verdict: Signal{Source: "openai", Existence: Alive, Weight: 0.8, Note: "recent posts"}, findings: []Finding{{Source: "openai", Field: "email", Value: "a@b.be", Confidence: 0.8}}}
	p := NewWith(j, w)
	res := p.Run(context.Background(), model.Record{Name: "X"}, Options{Sources: []string{"website", "openai"}}, NewBudget(1))
	if res.Verification.Verdict != model.VerdictActive || j.verdicts != 1 || j.extracts != 1 {
		t.Fatalf("judge path: %+v %+v", res.Verification, j)
	}
	var email *model.Suggestion
	for i := range res.Suggestions {
		if res.Suggestions[i].Field == "email" {
			email = &res.Suggestions[i]
		}
	}
	if email == nil || email.Kind != model.KindNew || email.Evidence[0].Source != "openai" {
		t.Fatalf("gap filling: %+v", res.Suggestions)
	}
}

func TestPipelineBudgetSkipsAndSkipRules(t *testing.T) {
	g := &fakeSource{name: "google", cost: 0.03, ready: true}
	p := NewWith(nil, g)
	res := p.Run(context.Background(), model.Record{Name: "X"}, Options{Sources: []string{"google"}}, NewBudget(0.01))
	if res.Verification.Verdict != model.VerdictSkipped || res.Verification.Skipped != "budget" || g.calls != 0 {
		t.Fatalf("budget: %+v", res.Verification)
	}
	res = p.Run(context.Background(), model.Record{Name: "VME Residentie", Source: map[string]any{"Rechtsvorm": "Vereniging van Mede-eigenaars"}}, Options{Sources: []string{"google"}}, NewBudget(1))
	if res.Verification.Skipped != "non_commercial" || g.calls != 0 {
		t.Fatalf("skip rule: %+v", res.Verification)
	}
	res = p.Run(context.Background(), model.Record{Name: "VME Residentie", Source: map[string]any{"Rechtsvorm": "Vereniging van Mede-eigenaars"}}, Options{Sources: []string{"google"}, Force: true}, NewBudget(1))
	if g.calls != 1 {
		t.Fatal("force should override the skip rule")
	}
	failing := &fakeSource{name: "website", ready: true, err: errors.New("dns")}
	p = NewWith(nil, failing)
	res = p.Run(context.Background(), model.Record{Name: "X"}, Options{Sources: []string{"website"}}, NewBudget(1))
	if res.Verification.Verdict != model.VerdictUnclear || res.Verification.Reason == "" {
		t.Fatalf("source failure must surface: %+v", res.Verification)
	}
	if avail := p.Available(); avail["website"] != true || avail["openai"] != false {
		t.Fatalf("available: %v", avail)
	}
	if r := p.Resolve([]string{"openai", "website", "bogus"}); len(r) != 1 || r[0] != "website" {
		t.Fatalf("resolve: %v", r)
	}
}

func TestPipelineTraceAndOnStep(t *testing.T) {
	g := &fakeSource{name: "google", cost: 0.03, ready: true, signals: []Signal{{Source: "google", Existence: Alive, Weight: 0.7, Note: "operational"}}, findings: []Finding{{Source: "google", Field: "phone", Value: "03 1", Confidence: 0.7}}}
	w := &fakeSource{name: "website", ready: true, signals: []Signal{{Source: "website", Existence: Alive, Weight: 0.5, Note: "live"}}, pages: []Page{{Text: "x"}}}
	j := &fakeJudge{findings: []Finding{{Source: "openai", Field: "email", Value: "a@b.be", Confidence: 0.8}}}
	var live []model.Step
	res := NewWith(j, g, w).Run(context.Background(), model.Record{Name: "X"}, Options{Sources: []string{"google", "website", "openai"}, OnStep: func(s model.Step) { live = append(live, s) }}, NewBudget(1))
	stages := []string{}
	for _, s := range res.Verification.Trace {
		stages = append(stages, s.Stage+":"+s.Status)
	}
	want := "google:done website:done judge:skipped extract:done reconcile:done"
	if got := strings.Join(stages, " "); got != want {
		t.Fatalf("trace %q want %q", got, want)
	}
	if res.Verification.Trace[0].Cost != 0.03 || res.Verification.Trace[0].Findings != 1 || !strings.Contains(res.Verification.Trace[0].Note, "operational") {
		t.Fatalf("google step: %+v", res.Verification.Trace[0])
	}
	if res.Verification.Trace[4].Findings != 2 || !strings.Contains(res.Verification.Trace[4].Note, "2 new") {
		t.Fatalf("reconcile step: %+v", res.Verification.Trace[4])
	}
	// Every started step is reported twice (running, then final), instantaneous ones too.
	if len(live) != 2*len(res.Verification.Trace) || live[0].Status != model.StepRunning || live[1].Status != model.StepDone {
		t.Fatalf("live steps: %d for %d trace entries", len(live), len(res.Verification.Trace))
	}
	skipped := NewWith(nil, g).Run(context.Background(), model.Record{Name: ""}, Options{Sources: []string{"google"}}, NewBudget(1))
	if len(skipped.Verification.Trace) != 1 || skipped.Verification.Trace[0].Status != model.StepSkipped {
		t.Fatalf("skipped trace: %+v", skipped.Verification.Trace)
	}
}
