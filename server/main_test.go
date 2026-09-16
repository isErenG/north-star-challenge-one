package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSchotenSchema(t *testing.T) {
	// Synthetic values, same field names as the supplied export. No private dataset fixture.
	input := "Ondernemingsnr,Maatschappelijke_naam,Ondernemingsnr_maatsch_zetel,KBO_Straat,KBO_Huisnr,KBO_Busnr,KBO_Postcode,KBO_Gemeente,longitude,latitude,Extra\n2285533695,Example,0719273014,Example Street,42,B,2900,Schoten,4.50,51.25,keep me\n"
	rows, err := parse(strings.NewReader(input), ".csv")
	if err != nil {
		t.Fatal(err)
	}
	r := rows[0]
	if r.Number != "2285533695" || r.Enterprise != "0719273014" || r.Kind != "establishment" {
		t.Fatalf("identifier mapping: %+v", r)
	}
	if r.Address != "Example Street 42 bus B, 2900 Schoten" || r.Source["Extra"] != "keep me" || r.Geometry == nil {
		t.Fatalf("lost source data: %+v", r)
	}
}
func TestFormats(t *testing.T) {
	cases := []struct{ ext, input string }{
		{".csv", "\ufeffnumber;name;address\n0123456749;\"One; Two\";Belgium\n"},
		{".json", `[{"number":"0123456749","name":"One; Two","address":"Belgium","custom":{"a":1}}]`},
		{".geojson", `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"number":"0123456749","name":"One; Two"},"geometry":{"type":"Point","coordinates":[4.5,51.2]}}]}`},
	}
	for _, c := range cases {
		t.Run(c.ext, func(t *testing.T) {
			rows, err := parse(strings.NewReader(c.input), c.ext)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Number != "0123456749" || rows[0].Name != "One; Two" {
				t.Fatalf("bad parsing: %+v", rows)
			}
			if c.ext == ".geojson" && rows[0].Geometry == nil {
				t.Fatal("geometry lost")
			}
		})
	}
}
func TestRejectsMalformedInput(t *testing.T) {
	for _, c := range []struct{ ext, input string }{{".csv", "name,name\nx,y"}, {".csv", "name,number\nx,y,z"}, {".json", "{}"}, {".json", "[]"}, {".json", `[{"name":"a"}] {}`}, {".geojson", `{"type":"FeatureCollection","features":[null]}`}, {".csv", "unknown\nx"}} {
		if _, err := parse(strings.NewReader(c.input), c.ext); err == nil {
			t.Errorf("accepted %s", c.input)
		}
	}
}
func TestNumberValidation(t *testing.T) {
	if !validNumber("0123.456.749") {
		t.Error("valid leading-zero enterprise number rejected")
	}
	if validNumber("0123456748") || validNumber("123") {
		t.Error("invalid identifier accepted")
	}
}
func TestRoundTripSourceAndGeometry(t *testing.T) {
	original := `[{"number":"0123456749","name":"Original","custom":"untouched","longitude":4.5,"latitude":51.2}]`
	rows, err := parse(strings.NewReader(original), ".json")
	if err != nil {
		t.Fatal(err)
	}
	rows[0].Name = "Edited"
	rows[0].Reviewed = true
	encoded, _ := json.Marshal(map[string]any{"records": rows})
	again, err := parse(bytes.NewReader(encoded), ".json")
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Name != "Edited" || again[0].Source["name"] != "Original" || again[0].Source["custom"] != "untouched" || !again[0].Reviewed || again[0].Geometry == nil {
		t.Fatalf("round trip lost data: %+v", again[0])
	}
}
func TestDuplicateAndPersistentJob(t *testing.T) {
	db.jobs = map[string]*Job{}
	db.dir = t.TempDir()
	db.slots = make(chan struct{}, 2)
	rows, _ := parse(strings.NewReader("number,name\n0123456749,A\n0123456749,B"), ".csv")
	j := &Job{ID: "test", Records: rows, Total: 2, State: "processing"}
	db.jobs[j.ID] = j
	db.slots <- struct{}{}
	process(j)
	if j.State != "done" || j.Progress != 2 {
		t.Fatalf("job failed: %+v", j)
	}
	for _, r := range j.Records {
		if !strings.Contains(strings.Join(r.Issues, ","), "Duplicate identifier") {
			t.Fatal("duplicate missed")
		}
	}
	validate(&j.Records[0])
	if !strings.Contains(strings.Join(j.Records[0].Issues, ","), "Duplicate identifier") {
		t.Fatal("editing cleared duplicate flag")
	}
	req := httptest.NewRequest("GET", "/api/jobs/test", nil)
	req.SetPathValue("id", "test")
	w := httptest.NewRecorder()
	getJob(w, req)
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
}

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGoogleCandidateNeverOverwritesSource(t *testing.T) {
	old := client
	defer func() { client = old }()
	t.Setenv("GOOGLE_MAPS_API_KEY", "test")
	client = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Goog-Api-Key") != "test" {
			t.Error("missing server key")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"places":[{"id":"candidate","displayName":{"text":"A different business"},"businessStatus":"CLOSED_PERMANENTLY","formattedAddress":"Other address"}]}`))}, nil
	})}
	row := Record{Name: "Original business", Address: "Original address", Status: "Normale toestand"}
	candidate, err := lookup(row)
	if err != "" || candidate.BusinessStatus != "CLOSED_PERMANENTLY" {
		t.Fatal(candidate, err)
	}
	if row.Name != "Original business" || row.Status != "Normale toestand" {
		t.Fatal("unconfirmed match changed source")
	}
	client = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("quota exceeded"))}, nil
	})}
	if p, err := lookup(row); p != nil || !strings.Contains(err, "429") {
		t.Fatal("API failure must stay unconfirmed")
	}
}
func TestNotificationRequestedOnce(t *testing.T) {
	old := client
	defer func() { client = old }()
	db.dir = t.TempDir()
	t.Setenv("RESEND_API_KEY", "test")
	t.Setenv("NOTIFY_FROM", "test@example.com")
	calls := 0
	client = &http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Idempotency-Key") != "kbo-job-notify" {
			t.Error("missing idempotency key")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"test"}`))}, nil
	})}
	j := &Job{ID: "notify", State: "done", Email: "user@example.com", Notification: "pending", Total: 3}
	sendNotification(j)
	sendNotification(j)
	if calls != 1 || j.Notification != "sent" {
		t.Fatalf("unexpected notification state: %d %s", calls, j.Notification)
	}
}
