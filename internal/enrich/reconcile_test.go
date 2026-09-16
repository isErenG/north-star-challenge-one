package enrich

import (
	"testing"

	"kbo-review/internal/model"
)

func TestVerdictRules(t *testing.T) {
	cases := []struct {
		name    string
		signals []Signal
		want    string
	}{
		{"strong closure wins", []Signal{{Source: "google", Existence: Closed, Weight: 0.9}, {Source: "website", Existence: Alive, Weight: 0.9}}, model.VerdictCeased},
		{"two live signals", []Signal{{Source: "google", Existence: Alive, Weight: 0.7}, {Source: "website", Existence: Alive, Weight: 0.5}}, model.VerdictActive},
		{"single strong live", []Signal{{Source: "website", Existence: Alive, Weight: 0.9}}, model.VerdictActive},
		{"weak only", []Signal{{Source: "website", Existence: Closed, Weight: 0.4}}, model.VerdictUnclear},
		{"nothing", nil, model.VerdictUnclear},
		{"live but contradicted", []Signal{{Source: "google", Existence: Alive, Weight: 0.7}, {Source: "website", Existence: Closed, Weight: 0.7}}, model.VerdictUnclear},
	}
	for _, c := range cases {
		if got, _ := verdict(c.signals); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestReconcile(t *testing.T) {
	rec := model.Record{Phone: "03 123 45 67", Email: "", Website: "https://www.example.be/"}
	findings := []Finding{
		{Source: "google", Field: "phone", Value: "+32 3 123 45 67", Confidence: 0.7},
		{Source: "website", Field: "phone", Value: "+32 3 123 45 67", Confidence: 0.6},
		{Source: "website", Field: "email", Value: "jan@example.be", Confidence: 0.8},
		{Source: "google", Field: "website", Value: "example.be", Confidence: 0.7},
		{Source: "google", Field: "address", Value: "Other street 1, 2900 Schoten", Confidence: 0.7},
		{Source: "trendstop", Field: "address", Value: "", Confidence: 0.9},
	}
	rec.Address = "Some street 2, 2900 Schoten"
	out := reconcile(rec, findings)
	got := map[string]model.Suggestion{}
	for _, s := range out {
		got[s.Field] = s
	}
	if s := got["phone"]; s.Kind != model.KindConfirmed || s.Confidence < 0.85 || len(s.Evidence) != 2 {
		t.Fatalf("phone: %+v", s)
	}
	if s := got["email"]; s.Kind != model.KindNew || s.Value != "jan@example.be" {
		t.Fatalf("email: %+v", s)
	}
	if s := got["website"]; s.Kind != model.KindConfirmed {
		t.Fatalf("website normalisation: %+v", s)
	}
	if s := got["address"]; s.Kind != model.KindDifferent || s.Current != rec.Address {
		t.Fatalf("address: %+v", s)
	}
	if out[0].Field != "address" || out[len(out)-1].Field != "website" {
		t.Fatalf("order: %v", out)
	}
}

func TestGapsAndNormalize(t *testing.T) {
	rec := model.Record{Phone: "03 123 45 67"}
	g := gaps(rec, []Finding{{Field: "email", Confidence: 0.5}})
	want := map[string]bool{"email": true, "website": true, "address": true, "activity": true}
	if len(g) != 4 {
		t.Fatalf("gaps: %v", g)
	}
	for _, f := range g {
		if !want[f] {
			t.Fatalf("unexpected gap %s", f)
		}
	}
	if Normalize("phone", "+32 (0)3 123.45.67") != "0031234567" && Normalize("phone", "+32 3 123 45 67") != "031234567" {
		t.Fatal("phone normalisation")
	}
	if Normalize("phone", "0032 3 123 45 67") != Normalize("phone", "03/123.45.67") {
		t.Fatal("international prefix normalisation")
	}
	if Similarity("Bakkerij Janssens BV", "Janssens Bakkerij") < 0.99 {
		t.Fatal("similarity ignores legal form and order")
	}
	if Similarity("Bakkerij Janssens", "Garage Peeters") != 0 {
		t.Fatal("unrelated names should not match")
	}
	if Containment("Colruyt Group NV", "Welcome to Colruyt Group, retail since 1928 with many stores and lots of other words here") != 1 {
		t.Fatal("containment should ignore page length")
	}
}

func TestBudgetAndLimiter(t *testing.T) {
	b := NewBudget(0.05)
	if !b.Reserve(0.03) || !b.Reserve(0.02) || b.Reserve(0.01) || !b.Reserve(0) {
		t.Fatalf("budget accounting wrong: %v", b.Spent())
	}
	c := NewFileCache(t.TempDir(), 0)
	c.Put("x", "k", map[string]int{"a": 1})
	var out map[string]int
	if c.Get("x", "k", &out) {
		t.Fatal("expired entry served")
	}
	c = NewFileCache(t.TempDir(), 1e9)
	c.Put("x", "k", map[string]int{"a": 1})
	if !c.Get("x", "k", &out) || out["a"] != 1 {
		t.Fatal("cache miss")
	}
}

func TestGenericEmailsAreDroppedAndAutoAccept(t *testing.T) {
	for _, e := range []string{"info@x.be", "Contact@x.be", "info.schoten@x.be", "no-reply@x.be", "onthaal@x.be", "bad"} {
		if !GenericEmail(e) {
			t.Errorf("%s should be generic", e)
		}
	}
	for _, e := range []string{"jan.peeters@x.be", "dr.springael@x.be", "information-desk@x.be"} {
		if GenericEmail(e) {
			t.Errorf("%s should be personal", e)
		}
	}
	rec := model.Record{}
	findings := []Finding{
		{Source: "website", Field: "email", Value: "info@x.be", Confidence: 0.9},
		{Source: "openai", Field: "email", Value: "info@x.be", Confidence: 0.9},
		{Source: "google", Field: "phone", Value: "+32 3 1", Confidence: 0.7},
		{Source: "website", Field: "phone", Value: "03 1", Confidence: 0.6},
		{Source: "openai", Field: "phone", Value: "+32 3 1", Confidence: 0.8},
		{Source: "google", Field: "website", Value: "https://x.be", Confidence: 0.95},
		{Source: "google", Field: "name", Value: "X BV", Confidence: 0.95},
		{Source: "openai", Field: "name", Value: "X BV", Confidence: 0.95},
	}
	out := reconcile(rec, findings)
	markAuto(out)
	got := map[string]model.Suggestion{}
	for _, s := range out {
		got[s.Field] = s
	}
	if _, ok := got["email"]; ok {
		t.Fatal("generic email must not be suggested")
	}
	if s := got["phone"]; !s.Auto || !s.Accepted || s.Confidence < 0.9 {
		t.Fatalf("phone with three agreeing sources should auto-accept: %+v", s)
	}
	if s := got["website"]; s.Auto {
		t.Fatalf("single source must not auto-accept: %+v", s)
	}
	if s := got["name"]; s.Auto {
		t.Fatalf("name must never auto-accept: %+v", s)
	}
}
