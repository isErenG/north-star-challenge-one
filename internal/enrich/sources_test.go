package enrich

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"kbo-review/internal/model"
)

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func respond(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func TestGoogleSignalsAndFindings(t *testing.T) {
	g := NewGoogle("key", 0.03, NoCache{})
	g.Limiter = NewLimiter(1000)
	g.HTTP = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Goog-Api-Key") != "key" || !strings.Contains(r.Header.Get("X-Goog-FieldMask"), "places.types") {
			t.Error("bad headers")
		}
		return respond(200, `{"places":[{"id":"p1","displayName":{"text":"Bakkerij Janssens"},"formattedAddress":"Zilverstraat 53, 2900 Schoten, Belgium","businessStatus":"CLOSED_PERMANENTLY","nationalPhoneNumber":"03 658 12 34","websiteUri":"https://janssens.be","googleMapsUri":"https://maps.google.com/?cid=1","types":["bakery","store"]}]}`), nil
	})}
	row := &Row{Record: model.Record{Name: "Bakkerij Janssens BV", Address: "Zilverstraat 53, 2900 Schoten"}}
	if err := g.Run(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	if s := signal(row, Closed); s == nil || s.Weight < 0.9 {
		t.Fatalf("closed signal: %+v", row.Signals)
	}
	if row.Place == nil || row.Place.BusinessStatus != "CLOSED_PERMANENTLY" {
		t.Fatal("place not kept for the existing UI")
	}
	fields := map[string]string{}
	for _, f := range row.Findings {
		fields[f.Field] = f.Value
	}
	if fields["phone"] != "03 658 12 34" || fields["website"] != "https://janssens.be" || fields["address"] != "Zilverstraat 53, 2900 Schoten" {
		t.Fatalf("findings: %+v", fields)
	}
	if len(row.Pages) != 1 || !strings.Contains(row.Pages[0].Text, "bakery") {
		t.Fatalf("types page: %+v", row.Pages)
	}

	// Name mismatch lowers weights instead of asserting a wrong business is alive.
	g.HTTP = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		return respond(200, `{"places":[{"id":"p2","displayName":{"text":"Frituur De Smet"},"businessStatus":"OPERATIONAL"}]}`), nil
	})}
	row = &Row{Record: model.Record{Name: "Bakkerij Janssens"}}
	_ = g.Run(context.Background(), row)
	if s := signal(row, Alive); s == nil || s.Weight > 0.3 || !strings.Contains(s.Note, "differs") {
		t.Fatalf("mismatch handling: %+v", row.Signals)
	}
	// A trade name in the source is what listings use; an address-only hit is no listing.
	g.HTTP = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "Frituur Den Deuzeld") {
			t.Errorf("query should use the trade name: %s", body)
		}
		return respond(200, `{"places":[{"id":"p3","displayName":{"text":"Eethuisstraat 2"},"formattedAddress":"Eethuisstraat 2, 2170 Schoten, Belgium","types":["street_address"]}]}`), nil
	})}
	row = &Row{Record: model.Record{Name: "De Cesuur", Address: "Eethuisstraat 2, 2170 Schoten", Source: map[string]any{"Commerciele_naam": "Frituur Den Deuzeld"}}}
	_ = g.Run(context.Background(), row)
	if s := signal(row, Unknown); s == nil || !strings.Contains(s.Note, "only the street address") || row.Place != nil || len(row.Findings) != 0 {
		t.Fatalf("address-only result must not count: %+v %+v", row.Signals, row.Findings)
	}
	g.HTTP = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) { return respond(429, "quota"), nil })}
	row = &Row{Record: model.Record{Name: "X"}}
	_ = g.Run(context.Background(), row)
	if s := signal(row, Unknown); s == nil || !strings.Contains(s.Note, "429") {
		t.Fatalf("api failure must be unknown: %+v", row.Signals)
	}
}

func TestOpenAIJudge(t *testing.T) {
	o := NewOpenAI("sk", "test-model", 0.002, NewFileCache(t.TempDir(), 1e9))
	o.Limiter = NewLimiter(1000)
	calls := 0
	o.HTTP = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer sk" || !strings.Contains(string(body), `"json_schema"`) || !strings.Contains(string(body), "test-model") {
			t.Error("bad request")
		}
		if calls == 1 {
			return respond(500, "boom"), nil
		}
		if strings.Contains(string(body), "existence_verdict") {
			return respond(200, `{"choices":[{"message":{"content":"{\"verdict\":\"closed\",\"confidence\":0.85,\"reason\":\"Site says closed.\",\"url\":\"https://x.be\"}"}}]}`), nil
		}
		return respond(200, `{"choices":[{"message":{"content":"{\"fields\":[{\"field\":\"email\",\"value\":\"info@x.be\",\"confidence\":0.9,\"url\":\"https://x.be/contact\",\"note\":\"contact page\"},{\"field\":\"activity\",\"value\":\"\",\"confidence\":0.1,\"url\":\"\",\"note\":\"\"}]}"}}]}`), nil
	})}
	row := &Row{Record: model.Record{Name: "X", Source: map[string]any{"Omschrijving_hoofdact_RSZ": "Bakkerij"}}, Pages: []Page{{URL: "https://x.be", Text: "closed"}}}
	if !strings.Contains(o.context(row), "registered activity: Bakkerij") {
		t.Fatal("activity not passed to the judge")
	}
	sig, err := o.Verdict(context.Background(), row)
	if err != nil || sig.Existence != Closed || sig.Weight != 0.85 || calls != 2 {
		t.Fatalf("verdict: %+v %v calls=%d", sig, err, calls)
	}
	findings, err := o.Extract(context.Background(), row, []string{"email", "activity"})
	if err != nil || len(findings) != 1 || findings[0].Field != "email" || findings[0].Confidence > 0.82 {
		t.Fatalf("extract: %+v %v", findings, err)
	}
	before := calls
	if sig2, err := o.Verdict(context.Background(), row); err != nil || sig2 != sig || calls != before {
		t.Fatalf("second verdict should come from cache: %+v %v calls=%d", sig2, err, calls-before)
	}
}
