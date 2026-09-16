package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kbo-review/internal/model"
)

func site(t *testing.T, pages map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
}

func runWebsite(t *testing.T, srv *httptest.Server, rec model.Record) *Row {
	t.Helper()
	w := NewWebsite(NoCache{})
	w.Limiter = NewLimiter(1000)
	rec.Website = srv.URL
	row := &Row{Record: rec}
	if err := w.Run(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	return row
}

func signal(row *Row, existence string) *Signal {
	for i := range row.Signals {
		if row.Signals[i].Existence == existence {
			return &row.Signals[i]
		}
	}
	return nil
}

func TestWebsiteExtractsContactsAndVAT(t *testing.T) {
	srv := site(t, map[string]string{
		"/":                  `<html><head><title>Bakkerij Janssens - Schoten</title><style>a{}</style></head><body><h1>Bakkerij Janssens</h1><a href="/nl/contacteer-ons/">Contacteer ons</a><script>var x="ignored@script.js"</script></body></html>`,
		"/nl/contacteer-ons": `<html><body>Bel ons: 03 658 12 34 of <a href="mailto:Piet@Janssens.be">mail</a>. BTW BE 0123.456.749. Zilverstraat 53, 2900 Schoten</body></html>`,
	})
	defer srv.Close()
	row := runWebsite(t, srv, model.Record{Number: "0123456749", Name: "Bakkerij Janssens BV"})
	if s := signal(row, Alive); s == nil || s.Weight < 0.9 {
		t.Fatalf("expected strong alive signal from VAT match: %+v", row.Signals)
	}
	var email, phone string
	for _, f := range row.Findings {
		switch f.Field {
		case "email":
			email = f.Value
		case "phone":
			phone = f.Value
		}
	}
	if email != "piet@janssens.be" || Normalize("phone", phone) != "036581234" {
		t.Fatalf("contacts: %q %q %+v", email, phone, row.Findings)
	}
	if len(row.Pages) != 2 || !strings.Contains(row.Pages[1].Text, "Zilverstraat") {
		t.Fatalf("pages: %+v", row.Pages)
	}
}

func TestWebsiteClosureParkedAndDead(t *testing.T) {
	closed := site(t, map[string]string{"/": `<html><title>Garage Peeters</title><body>Garage Peeters is definitief gesloten sinds 2025. Bedankt!</body></html>`})
	defer closed.Close()
	row := runWebsite(t, closed, model.Record{Number: "0123456749", Name: "Garage Peeters"})
	if s := signal(row, Closed); s == nil || s.Weight < 0.7 {
		t.Fatalf("closure notice missed: %+v", row.Signals)
	}
	parked := site(t, map[string]string{"/": `<html><title>Domain for sale</title><body>This domain is for sale. Buy this domain today.</body></html>`})
	defer parked.Close()
	row = runWebsite(t, parked, model.Record{Name: "Garage Peeters"})
	if s := signal(row, Closed); s == nil || !strings.Contains(s.Note, "parked") {
		t.Fatalf("parked page missed: %+v", row.Signals)
	}
	dead := site(t, map[string]string{})
	defer dead.Close()
	row = runWebsite(t, dead, model.Record{Name: "Garage Peeters"})
	if s := signal(row, Closed); s == nil || s.Weight != 0.4 || !strings.Contains(s.Note, "404") {
		t.Fatalf("dead site: %+v", row.Signals)
	}
	row = &Row{Record: model.Record{Name: "No site"}}
	_ = NewWebsite(NoCache{}).Run(context.Background(), row)
	if s := signal(row, Unknown); s == nil {
		t.Fatalf("missing website should be unknown: %+v", row.Signals)
	}
}

func TestWebsiteFromMismatchedListingIsDiscounted(t *testing.T) {
	srv := site(t, map[string]string{"/": `<html><title>De Riddershoeve</title><body>Welkom bij De Riddershoeve. info@riddershoeve.be 03 658 44 26</body></html>`})
	defer srv.Close()
	w := NewWebsite(NoCache{})
	w.Limiter = NewLimiter(1000)
	row := &Row{Record: model.Record{Number: "0123456749", Name: "BELHOEVE"}, Findings: []Finding{{Source: "google", Field: "website", Value: srv.URL, Confidence: 0.28}}}
	if err := w.Run(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	for _, f := range row.Findings {
		if f.Source == "website" && f.Confidence > 0.35 {
			t.Fatalf("finding from an unverified site should be discounted: %+v", f)
		}
	}
	if s := signal(row, Unknown); s == nil || !strings.Contains(s.Note, "another business") {
		t.Fatalf("expected a caution note: %+v", row.Signals)
	}
}
