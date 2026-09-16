package enrich

import (
	"sort"
	"strings"

	"kbo-review/internal/model"
)

// verdict applies the existence rules. It returns unclear when the evidence
// is thin so the judge can be consulted.
func verdict(signals []Signal) (string, string) {
	var alive, closedMax float64
	var closedNote, aliveNote string
	for _, s := range signals {
		switch s.Existence {
		case Alive:
			alive += s.Weight
			if aliveNote == "" || s.Weight > 0.6 {
				aliveNote = s.Source + ": " + s.Note
			}
		case Closed:
			if s.Weight > closedMax {
				closedMax = s.Weight
				closedNote = s.Source + ": " + s.Note
			}
		}
	}
	if closedMax >= 0.8 {
		return model.VerdictCeased, closedNote
	}
	if alive >= 1.0 && closedMax < 0.6 {
		return model.VerdictActive, aliveNote
	}
	if alive >= 0.7 && closedMax == 0 {
		return model.VerdictActive, aliveNote
	}
	return model.VerdictUnclear, summarise(signals)
}

func summarise(signals []Signal) string {
	if len(signals) == 0 {
		return "No source could confirm or deny that this business still operates."
	}
	parts := make([]string, 0, len(signals))
	for _, s := range signals {
		parts = append(parts, s.Source+": "+s.Note)
	}
	return strings.Join(parts, "; ")
}

// currentValue reads the baseline value for a field from the record.
func currentValue(r model.Record, field string) string {
	switch field {
	case "name":
		return r.Name
	case "address":
		return r.Address
	case "phone":
		return r.Phone
	case "email":
		return r.Email
	case "website":
		return r.Website
	case "status":
		return r.Status
	}
	return ""
}

// reconcile groups findings per field and turns the best-supported value into
// a suggestion. Values equal to the baseline become "confirmed".
func reconcile(r model.Record, findings []Finding) []model.Suggestion {
	byField := map[string][]Finding{}
	for _, f := range findings {
		if strings.TrimSpace(f.Value) == "" {
			continue
		}
		if f.Field == "email" && GenericEmail(f.Value) {
			continue
		}
		byField[f.Field] = append(byField[f.Field], f)
	}
	var out []model.Suggestion
	for field, list := range byField {
		type cand struct {
			value    string
			score    float64
			evidence []model.Evidence
		}
		cands := map[string]*cand{}
		for _, f := range list {
			key := Normalize(field, f.Value)
			c := cands[key]
			if c == nil {
				c = &cand{value: strings.TrimSpace(f.Value)}
				cands[key] = c
			}
			// Agreement between sources compounds: 1 - Π(1 - confidence).
			c.score = 1 - (1-c.score)*(1-f.Confidence)
			c.evidence = append(c.evidence, model.Evidence{Source: f.Source, URL: f.URL, Note: f.Note})
		}
		var best *cand
		for _, c := range cands {
			if best == nil || c.score > best.score || (c.score == best.score && c.value < best.value) {
				best = c
			}
		}
		if best == nil || best.score < 0.3 {
			continue
		}
		cur := currentValue(r, field)
		kind := model.KindNew
		if cur != "" {
			if Normalize(field, cur) == Normalize(field, best.value) {
				kind = model.KindConfirmed
			} else {
				kind = model.KindDifferent
			}
		}
		out = append(out, model.Suggestion{Field: field, Current: cur, Value: best.value, Kind: kind, Confidence: round2(best.score), Evidence: best.evidence})
	}
	sort.Slice(out, func(i, j int) bool { return fieldRank(out[i].Field) < fieldRank(out[j].Field) })
	return out
}

// AutoThreshold is the confidence needed for automatic acceptance.
const AutoThreshold = 0.9

// autoFields are the only fields ever applied automatically. Name, status and
// activity always wait for a human.
var autoFields = map[string]bool{"phone": true, "email": true, "website": true, "address": true}

// markAuto flags suggestions that are safe to apply without review: a contact
// field, confidence at or above the threshold, and at least two independent
// sources agreeing on the value.
func markAuto(suggestions []model.Suggestion) {
	for i := range suggestions {
		sg := &suggestions[i]
		if !autoFields[sg.Field] || sg.Kind == model.KindConfirmed || sg.Confidence < AutoThreshold {
			continue
		}
		sources := map[string]bool{}
		for _, e := range sg.Evidence {
			sources[e.Source] = true
		}
		if len(sources) >= 2 {
			sg.Auto = true
			sg.Accepted = true
		}
	}
}

func fieldRank(f string) int {
	for i, w := range []string{"status", "name", "address", "phone", "email", "website", "activity"} {
		if w == f {
			return i
		}
	}
	return 99
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func round4(f float64) float64 { return float64(int(f*10000+0.5)) / 10000 }

// gaps lists wanted fields that have no finding of confidence >= 0.6 and no
// baseline value, or whose findings disagree with the baseline.
func gaps(r model.Record, findings []Finding) []string {
	best := map[string]float64{}
	for _, f := range findings {
		if f.Confidence > best[f.Field] {
			best[f.Field] = f.Confidence
		}
	}
	var out []string
	for _, field := range WantedFields {
		if best[field] >= 0.6 {
			continue
		}
		if currentValue(r, field) == "" || field == "activity" {
			out = append(out, field)
		}
	}
	return out
}
