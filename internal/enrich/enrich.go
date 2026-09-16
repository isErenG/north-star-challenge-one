// Package enrich verifies records against real-world sources. The uploaded
// registry data is treated as a set of claims; sources supply evidence; the
// pipeline returns a verdict and per-field suggestions. It never mutates the
// record it is given.
package enrich

import (
	"context"

	"kbo-review/internal/model"
)

// Existence signal values.
const (
	Alive   = "alive"
	Closed  = "closed"
	Unknown = "unknown"
)

// Fields the pipeline tries to establish.
var WantedFields = []string{"phone", "email", "website", "address", "activity"}

// Signal is evidence about whether the business still exists.
type Signal struct {
	Source    string
	Existence string
	Weight    float64
	Note      string
	URL       string
}

// Finding is field-level evidence from one source.
type Finding struct {
	Source     string
	Field      string
	Value      string
	URL        string
	Note       string
	Confidence float64
}

// Page is fetched text kept for the judge.
type Page struct {
	URL   string
	Title string
	Text  string
}

// Row is the working state for one record during a run.
type Row struct {
	Record   model.Record
	Signals  []Signal
	Findings []Finding
	Pages    []Page
	Place    *model.Place
	Cost     float64
	Sources  []string
}

// Website returns the best-known website for the row: the record's own, or
// one a source found.
func (r *Row) Website() string {
	if r.Record.Website != "" {
		return r.Record.Website
	}
	for _, f := range r.Findings {
		if f.Field == "website" && f.Value != "" {
			return f.Value
		}
	}
	return ""
}

// Source is one external provider.
type Source interface {
	Name() string
	Ready() bool
	CostEUR() float64
	Run(ctx context.Context, row *Row) error
}

// Judge is the language-model step, used only for unclear rows and gaps.
type Judge interface {
	Ready() bool
	CostEUR() float64
	Verdict(ctx context.Context, row *Row) (Signal, error)
	Extract(ctx context.Context, row *Row, want []string) ([]Finding, error)
}

// Options control one job's run.
type Options struct {
	Sources     []string
	BudgetEUR   float64
	Concurrency int
	Force       bool // run contact collection even when likely ceased
	// OnStep, when set, receives every trace step as it starts and ends so
	// callers can show live progress. Steps with the same Stage replace the
	// earlier "running" entry.
	OnStep func(model.Step)
}

// Result is what a run produces for one record.
type Result struct {
	Verification model.Verification
	Suggestions  []model.Suggestion
	Place        *model.Place
}
