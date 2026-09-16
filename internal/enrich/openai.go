package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kbo-review/internal/model"
)

const openAIEndpoint = "https://api.openai.com/v1/chat/completions"

// OpenAI is the judge: it reads collected evidence and page text and answers
// with structured JSON. It never browses on its own.
type OpenAI struct {
	Key     string
	Model   string
	Cost    float64
	HTTP    *http.Client
	Limiter *Limiter
	Cache   Cache
}

func NewOpenAI(key, model string, costEUR float64, cache Cache) *OpenAI {
	if cache == nil {
		cache = NoCache{}
	}
	return &OpenAI{Key: key, Model: model, Cost: costEUR, HTTP: &http.Client{Timeout: 45 * time.Second}, Limiter: NewLimiter(5), Cache: cache}
}

func (o *OpenAI) Ready() bool      { return o != nil && o.Key != "" }
func (o *OpenAI) CostEUR() float64 { return o.Cost }

const maxContext = 24000

func (o *OpenAI) context(row *Row) string {
	var b strings.Builder
	r := row.Record
	fmt.Fprintf(&b, "REGISTRY RECORD (may be outdated)\nregistered name: %s\ntrade name: %s\nlegal form: %s\nenterprise number: %s\naddress: %s\nregistered status: %s\nphone: %s\nemail: %s\nwebsite: %s\n", r.Name, orNone(TradeName(r)), orNone(LegalForm(r)), r.Number, r.Address, r.Status, r.Phone, r.Email, r.Website)
	if act := activityOf(r); act != "" {
		fmt.Fprintf(&b, "registered activity: %s\n", act)
	}
	b.WriteString("\nSIGNALS\n")
	for _, s := range row.Signals {
		fmt.Fprintf(&b, "- %s: %s (%s, weight %.1f) %s\n", s.Source, s.Existence, s.Note, s.Weight, s.URL)
	}
	b.WriteString("\nFINDINGS\n")
	for _, f := range row.Findings {
		fmt.Fprintf(&b, "- %s %s=%q (%.1f) %s\n", f.Source, f.Field, f.Value, f.Confidence, f.URL)
	}
	b.WriteString("\nPAGES\n")
	for _, p := range row.Pages {
		if b.Len() > maxContext {
			break
		}
		fmt.Fprintf(&b, "## %s — %s\n%s\n", p.URL, p.Title, truncate(p.Text, maxContext-b.Len()))
	}
	return b.String()
}

// activityOf reads a NACE description from common source columns.
func activityOf(r model.Record) string {
	for _, k := range []string{"Omschrijving_hoofdact_BTW", "Omschrijving_hoofdact_RSZ", "activity", "nace_description", "NACE_hoofdact_BTW", "NACE_hoofdact_RSZ"} {
		if v, ok := r.Source[k]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

type verdictOut struct {
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	URL        string  `json:"url"`
}

type extractOut struct {
	Fields []struct {
		Field      string  `json:"field"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
		URL        string  `json:"url"`
		Note       string  `json:"note"`
	} `json:"fields"`
}

func (o *OpenAI) Verdict(ctx context.Context, row *Row) (Signal, error) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"verdict":    map[string]any{"type": "string", "enum": []string{"alive", "closed", "unknown"}},
			"confidence": map[string]any{"type": "number"},
			"reason":     map[string]any{"type": "string"},
			"url":        map[string]any{"type": "string"},
		},
		"required": []string{"verdict", "confidence", "reason", "url"},
	}
	system := "You audit Belgian business registry records against real-world evidence. The registry record may be stale. Decide from the SIGNALS, FINDINGS and PAGES whether THIS enterprise still operates.\n\nIdentity rules: a listing or website belongs to this enterprise only if it shows the same enterprise/VAT number, or its name matches the registered name or trade name, or the registered name is a person and the listing is plausibly that person's practice or shop (a doctor, dentist, lawyer, sole trader). A DIFFERENT business at the same address is NOT evidence this enterprise operates; it is weak evidence it moved or ceased. Never report a different business's details as this enterprise's.\n\nAnswer 'alive' only when evidence tied to this enterprise shows current activity. Answer 'closed' only with explicit evidence such as a closure notice, a permanently-closed listing for this enterprise, or a clear successor at the address. Otherwise 'unknown'. Reason in one sentence a municipal clerk can verify, citing the URL."
	var out verdictOut
	if err := o.call(ctx, system, o.context(row), "existence_verdict", schema, &out); err != nil {
		return Signal{}, err
	}
	sig := Signal{Source: "openai", Existence: out.Verdict, Weight: clamp(out.Confidence), Note: out.Reason, URL: out.URL}
	if sig.Existence == Unknown {
		sig.Weight = 0
	}
	return sig, nil
}

func (o *OpenAI) Extract(ctx context.Context, row *Row, want []string) ([]Finding, error) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"fields": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"properties": map[string]any{
						"field":      map[string]any{"type": "string", "enum": []string{"phone", "email", "website", "address", "activity", "name"}},
						"value":      map[string]any{"type": "string"},
						"confidence": map[string]any{"type": "number"},
						"url":        map[string]any{"type": "string"},
						"note":       map[string]any{"type": "string"},
					},
					"required": []string{"field", "value", "confidence", "url", "note"},
				},
			},
		},
		"required": []string{"fields"},
	}
	system := fmt.Sprintf("You extract verified contact details for one Belgian enterprise from the evidence provided. Only report values that appear in PAGES or FINDINGS; never invent. Report a value only if the page or listing it comes from belongs to THIS enterprise: same enterprise/VAT number, or a name matching the registered name or trade name, or a person's name matching a sole trader's practice. If the evidence is about a different business at the same address, report nothing for that source. Wanted fields: %s. For 'activity' give the most fitting NACE-BEL code and short description, e.g. '56.101 Restaurants', based on what this enterprise actually does. Cite the URL each value came from. Confidence 0-1. Omit fields you cannot support.", strings.Join(want, ", "))
	var out extractOut
	if err := o.call(ctx, system, o.context(row), "contact_extraction", schema, &out); err != nil {
		return nil, err
	}
	var findings []Finding
	for _, f := range out.Fields {
		if strings.TrimSpace(f.Value) == "" {
			continue
		}
		findings = append(findings, Finding{Source: "openai", Field: f.Field, Value: strings.TrimSpace(f.Value), URL: f.URL, Note: f.Note, Confidence: clamp(f.Confidence) * 0.9})
	}
	return findings, nil
}

// call sends one structured request. Identical evidence yields an identical
// answer at temperature 0, so responses are cached by prompt hash.
func (o *OpenAI) call(ctx context.Context, system, user, name string, schema map[string]any, out any) error {
	key := o.Model + "|" + name + "|" + system + "|" + user
	var cached json.RawMessage
	if o.Cache.Get("openai", key, &cached) && json.Unmarshal(cached, out) == nil {
		return nil
	}
	if err := o.Limiter.Wait(ctx, "openai"); err != nil {
		return err
	}
	payload := map[string]any{
		"model":       o.Model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": name, "strict": true, "schema": schema},
		},
	}
	body, _ := json.Marshal(payload)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+o.Key)
		res, err := o.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if res.StatusCode == 429 || res.StatusCode >= 500 {
			lastErr = fmt.Errorf("openai returned %d", res.StatusCode)
			time.Sleep(time.Second)
			continue
		}
		if res.StatusCode != 200 {
			return fmt.Errorf("openai returned %d: %s", res.StatusCode, truncate(string(raw), 200))
		}
		var envelope struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(raw, &envelope) != nil || len(envelope.Choices) == 0 {
			return errors.New("openai returned an unreadable response")
		}
		content := []byte(envelope.Choices[0].Message.Content)
		if err := json.Unmarshal(content, out); err != nil {
			return err
		}
		o.Cache.Put("openai", key, json.RawMessage(content))
		return nil
	}
	return lastErr
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func clamp(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
