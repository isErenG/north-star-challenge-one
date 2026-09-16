package enrich

import (
	"context"
	"reflect"
	"testing"

	"kbo-review/internal/model"
)

func TestCompanywebDemoCannotAffectVerification(t *testing.T) {
	p := NewWith(nil)
	for _, rec := range []model.Record{
		{Name: "Coja", Kind: "enterprise", Number: "0725606718"},
		{Name: "Coja location", Kind: "establishment", Number: "2123456789", Enterprise: "BE 0725.606.718"},
		{Name: "Other", Kind: "enterprise", Number: "0123456789"},
		{Name: "Coja", Kind: "establishment", Number: "0725606718"},
	} {
		base := p.Run(context.Background(), rec, Options{}, NewBudget(0))
		demo := p.Run(context.Background(), rec, Options{Sources: []string{"companyweb_demo"}}, NewBudget(0))
		if demo.Verification.Verdict != base.Verification.Verdict || demo.Verification.Reason != base.Verification.Reason || demo.Verification.Cost != 0 || len(demo.Verification.Sources) != 0 || !reflect.DeepEqual(demo.Suggestions, base.Suggestions) {
			t.Fatalf("demo changed verification: %+v", demo)
		}
		steps := demo.Verification.Trace
		step := steps[len(steps)-2]
		if step.Stage != "companyweb_demo" || steps[len(steps)-1].Stage != "reconcile" {
			t.Fatalf("unexpected order: %+v", steps)
		}
		wantSnapshot := rec.Enterprise != "" || rec.Kind == "enterprise" && rec.Number == "0725606718"
		if (step.Demo != nil) != wantSnapshot || step.Findings != 0 {
			t.Fatalf("wrong fixture match for %+v: %+v", rec, step)
		}
	}
	if got := p.Resolve([]string{"companyweb_demo"}); len(got) != 1 || got[0] != "companyweb_demo" {
		t.Fatalf("demo not selectable: %v", got)
	}
	for _, key := range p.Resolve(nil) {
		if key == "companyweb_demo" {
			t.Fatal("demo enabled by default")
		}
	}
}
