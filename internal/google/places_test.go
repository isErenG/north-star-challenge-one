package google

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"kbo-review/internal/model"
)

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCandidateNeverOverwritesSource(t *testing.T) {
	c := &Client{Key: "test", HTTP: &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Goog-Api-Key") != "test" {
			t.Error("missing server key")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"places":[{"id":"candidate","displayName":{"text":"A different business"},"businessStatus":"CLOSED_PERMANENTLY","formattedAddress":"Other address"}]}`))}, nil
	})}}
	row := model.Record{Name: "Original business", Address: "Original address", Status: "Normale toestand"}
	candidate, reason := c.Lookup(row)
	if reason != "" || candidate.BusinessStatus != "CLOSED_PERMANENTLY" {
		t.Fatal(candidate, reason)
	}
	if row.Name != "Original business" || row.Status != "Normale toestand" {
		t.Fatal("unconfirmed match changed source")
	}
	c.HTTP = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("quota exceeded"))}, nil
	})}
	if p, reason := c.Lookup(row); p != nil || !strings.Contains(reason, "429") {
		t.Fatal("API failure must stay unconfirmed")
	}
	if p, reason := c.Lookup(model.Record{}); p != nil || reason == "" {
		t.Fatal("lookup without name/address must be skipped")
	}
}
