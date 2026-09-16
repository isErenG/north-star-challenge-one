package enrich

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kbo-review/internal/config"
	"kbo-review/internal/model"
)

// DefaultSources is the run order when the upload does not choose.
var DefaultSources = []string{"google", "website", "openai"}

// AllSources lists every source key the API understands, in run order.
var AllSources = []string{"google", "website", "databe", "goldenpages", "trendstop", "vkbo", "openai"}

// Pipeline runs sources in order and judges the result.
type Pipeline struct {
	sources map[string]Source
	judge   Judge
}

// New wires every source from configuration.
func New(cfg config.Config) *Pipeline {
	cache := NewFileCache(filepath.Join(cfg.DataDir, "cache"), 30*24*time.Hour)
	p := &Pipeline{sources: map[string]Source{}}
	p.sources["website"] = NewWebsite(cache)
	p.sources["google"] = NewGoogle(cfg.GoogleKey, cfg.GoogleCostEUR, cache)
	p.sources["databe"] = NewDataBe(cfg.DataBeToken, cache)
	p.sources["vkbo"] = NewVKBO(cache)
	for _, d := range []string{"goldenpages", "trendstop"} {
		p.sources[d] = NewDirectory(d, slices.Contains(cfg.Directories, d), cache)
	}
	p.judge = NewOpenAI(cfg.OpenAIKey, cfg.OpenAIModel, cfg.OpenAICostEUR, cache)
	return p
}

// NewWith builds a pipeline from explicit parts (tests).
func NewWith(judge Judge, sources ...Source) *Pipeline {
	p := &Pipeline{sources: map[string]Source{}, judge: judge}
	for _, s := range sources {
		p.sources[s.Name()] = s
	}
	return p
}

// Available reports which sources can run right now.
func (p *Pipeline) Available() map[string]bool {
	out := map[string]bool{}
	for _, key := range AllSources {
		if key == "openai" {
			out[key] = p.judge != nil && p.judge.Ready()
			continue
		}
		s, ok := p.sources[key]
		out[key] = ok && s.Ready()
	}
	return out
}

// Resolve filters requested sources to known, ready ones in run order.
func (p *Pipeline) Resolve(requested []string) []string {
	if len(requested) == 0 {
		requested = DefaultSources
	}
	avail := p.Available()
	var out []string
	for _, key := range AllSources {
		if slices.Contains(requested, key) && avail[key] {
			out = append(out, key)
		}
	}
	return out
}

// nonCommercial legal forms never have listings; verifying them wastes budget.
var nonCommercial = []string{"vereniging van mede-eigenaars", "vereniging van medeëigenaars", "association des copropriétaires"}

func skipReason(r model.Record) string {
	form := strings.ToLower(fmt.Sprint(r.Source["Rechtsvorm"]))
	for _, nc := range nonCommercial {
		if strings.Contains(form, nc) {
			return "non_commercial"
		}
	}
	if strings.TrimSpace(r.Name) == "" {
		return "no_name"
	}
	return ""
}

// tracer records steps and forwards them to the caller's OnStep.
type tracer struct {
	steps  []model.Step
	onStep func(model.Step)
}

func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (t *tracer) start(stage string) int {
	s := model.Step{Stage: stage, Status: model.StepRunning, StartedAt: stamp()}
	t.steps = append(t.steps, s)
	if t.onStep != nil {
		t.onStep(s)
	}
	return len(t.steps) - 1
}

func (t *tracer) end(i int, status, note string, cost float64, findings int) {
	s := &t.steps[i]
	s.Status, s.Note, s.Cost, s.Findings, s.EndedAt = status, note, round4(cost), findings, stamp()
	if t.onStep != nil {
		t.onStep(*s)
	}
}

// add records an instantaneous step.
func (t *tracer) add(stage, status, note string) {
	i := t.start(stage)
	t.end(i, status, note, 0, 0)
}

// sourceNote summarises what one source contributed.
func sourceNote(row *Row, key string, before int) string {
	var notes []string
	for _, s := range row.Signals[before:] {
		if s.Source == key && s.Note != "" {
			notes = append(notes, s.Note)
		}
	}
	n := 0
	for _, f := range row.Findings {
		if f.Source == key {
			n++
		}
	}
	note := strings.Join(notes, "; ")
	if n > 0 {
		note = strings.TrimSuffix(note, ".") + fmt.Sprintf(" · %d detail(s) found", n)
	}
	return strings.TrimPrefix(strings.TrimSpace(note), "· ")
}

func countFindings(row *Row, key string) int {
	n := 0
	for _, f := range row.Findings {
		if f.Source == key {
			n++
		}
	}
	return n
}

// Run verifies one record. It never returns an error: every failure becomes
// part of the verification so the reviewer sees what happened.
func (p *Pipeline) Run(ctx context.Context, rec model.Record, opts Options, budget *Budget) Result {
	now := time.Now().UTC().Format(time.RFC3339)
	tr := &tracer{onStep: opts.OnStep}
	if reason := skipReason(rec); reason != "" && !opts.Force {
		tr.add("reconcile", model.StepSkipped, skipText(reason))
		return Result{Verification: model.Verification{Verdict: model.VerdictSkipped, Skipped: reason, Reason: skipText(reason), Sources: []string{}, CheckedAt: now, Trace: tr.steps}}
	}
	row := &Row{Record: rec}
	useJudge := slices.Contains(opts.Sources, "openai") && p.judge != nil && p.judge.Ready()
	over := false
	for _, key := range opts.Sources {
		s, ok := p.sources[key]
		if !ok || !s.Ready() {
			continue
		}
		if !budget.Reserve(s.CostEUR()) {
			over = true
			tr.add(key, model.StepSkipped, "Budget reached")
			break
		}
		i := tr.start(key)
		before := len(row.Signals)
		row.Cost += s.CostEUR()
		row.Sources = append(row.Sources, key)
		if err := s.Run(ctx, row); err != nil {
			row.Signals = append(row.Signals, Signal{Source: key, Existence: Unknown, Note: "Source failed: " + err.Error()})
			tr.end(i, model.StepFailed, err.Error(), s.CostEUR(), countFindings(row, key))
			continue
		}
		tr.end(i, model.StepDone, sourceNote(row, key, before), s.CostEUR(), countFindings(row, key))
	}
	v, reason := verdict(row.Signals)
	if v == model.VerdictUnclear && useJudge && len(row.Signals) > 0 && !over {
		if budget.Reserve(p.judge.CostEUR()) {
			i := tr.start("judge")
			row.Cost += p.judge.CostEUR()
			row.Sources = append(row.Sources, "openai")
			if sig, err := p.judge.Verdict(ctx, row); err == nil {
				row.Signals = append(row.Signals, sig)
				v, reason = verdict(row.Signals)
				// The judge is consulted precisely because the rules could not
				// decide; a confident answer settles it.
				if v == model.VerdictUnclear && sig.Weight >= 0.6 {
					switch sig.Existence {
					case Alive:
						v = model.VerdictActive
					case Closed:
						v = model.VerdictCeased
					}
					reason = "openai: " + sig.Note
				}
				tr.end(i, model.StepDone, fmt.Sprintf("%s (%.0f%%): %s", sig.Existence, sig.Weight*100, sig.Note), p.judge.CostEUR(), 0)
			} else {
				row.Signals = append(row.Signals, Signal{Source: "openai", Existence: Unknown, Note: "Judge failed: " + err.Error()})
				tr.end(i, model.StepFailed, err.Error(), p.judge.CostEUR(), 0)
			}
		} else {
			over = true
			tr.add("judge", model.StepSkipped, "Budget reached")
		}
	} else if v != model.VerdictUnclear {
		tr.add("judge", model.StepSkipped, "Rules were decisive; no AI needed")
	} else if !useJudge {
		tr.add("judge", model.StepSkipped, "AI reconciliation not enabled")
	}
	if v != model.VerdictCeased || opts.Force {
		if want := gaps(rec, row.Findings); len(want) > 0 && useJudge && len(row.Pages) > 0 && !over {
			if budget.Reserve(p.judge.CostEUR()) {
				i := tr.start("extract")
				row.Cost += p.judge.CostEUR()
				if !slices.Contains(row.Sources, "openai") {
					row.Sources = append(row.Sources, "openai")
				}
				if extra, err := p.judge.Extract(ctx, row, want); err == nil {
					row.Findings = append(row.Findings, extra...)
					tr.end(i, model.StepDone, fmt.Sprintf("Looked for %s", strings.Join(want, ", ")), p.judge.CostEUR(), len(extra))
				} else {
					tr.end(i, model.StepFailed, err.Error(), p.judge.CostEUR(), 0)
				}
			} else {
				over = true
				tr.add("extract", model.StepSkipped, "Budget reached")
			}
		} else if len(want) == 0 {
			tr.add("extract", model.StepSkipped, "Nothing missing")
		} else if !useJudge {
			tr.add("extract", model.StepSkipped, "AI reconciliation not enabled")
		} else if len(row.Pages) == 0 {
			tr.add("extract", model.StepSkipped, "No page text to read")
		}
	} else {
		tr.add("extract", model.StepSkipped, "Likely ceased; contact collection stopped")
	}
	ver := model.Verification{Verdict: v, Reason: reason, Sources: append([]string{}, row.Sources...), Cost: round4(row.Cost), CheckedAt: now}
	if len(row.Sources) == 0 {
		ver.Verdict = model.VerdictSkipped
		ver.Skipped = "budget"
		ver.Reason = "Budget reached before any source could run."
		if !over {
			ver.Skipped = "not_configured"
			ver.Reason = "No verification source is configured."
		}
	} else if over {
		ver.Reason = strings.TrimSuffix(ver.Reason, ".") + ". Budget reached; later steps were skipped."
	}
	ri := tr.start("reconcile")
	suggestions := reconcile(rec, row.Findings)
	if v == model.VerdictCeased {
		suggestions = append([]model.Suggestion{{Field: "status", Current: rec.Status, Value: "Likely ceased", Kind: model.KindDifferent, Confidence: 0.8, Evidence: evidenceFor(row.Signals, Closed)}}, suggestions...)
	}
	markAuto(suggestions)
	var newN, diffN, confN, autoN int
	for _, sg := range suggestions {
		if sg.Auto {
			autoN++
		}
		switch sg.Kind {
		case model.KindNew:
			newN++
		case model.KindDifferent:
			diffN++
		default:
			confN++
		}
	}
	tr.end(ri, model.StepDone, fmt.Sprintf("%d new, %d different, %d confirmed · %d auto-verified", newN, diffN, confN, autoN), 0, len(suggestions))
	ver.Trace = tr.steps
	return Result{Verification: ver, Suggestions: suggestions, Place: row.Place}
}

func evidenceFor(signals []Signal, existence string) []model.Evidence {
	var out []model.Evidence
	for _, s := range signals {
		if s.Existence == existence {
			out = append(out, model.Evidence{Source: s.Source, URL: s.URL, Note: s.Note})
		}
	}
	return out
}

func skipText(reason string) string {
	switch reason {
	case "non_commercial":
		return "Co-owner associations have no public listings; skipped to save budget."
	case "no_name":
		return "The record has no business name to search for."
	}
	return "Skipped."
}
