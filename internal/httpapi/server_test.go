package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"kbo-review/internal/config"
	"kbo-review/internal/google"
	"kbo-review/internal/jobs"
	"kbo-review/internal/notify"
	"kbo-review/internal/store"
)

func newServer(t *testing.T) http.Handler {
	t.Helper()
	svc, err := jobs.New(store.NewMemory(), google.New(""), notify.New("", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	site := fstest.MapFS{
		"index.html":                 {Data: []byte("<html>app</html>")},
		"index.html.gz":              {Data: []byte("gz-index")},
		"_app/immutable/chunk.js":    {Data: []byte("js")},
		"_app/immutable/chunk.js.br": {Data: []byte("br-js")},
	}
	return New(config.Config{MapboxToken: "pk.test"}, svc, site)
}

func do(h http.Handler, method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Host = "example.test"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if v := headers["Content-Length"]; v != "" {
		req.ContentLength, _ = strconv.ParseInt(v, 10, 64)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func upload(t *testing.T, h http.Handler, csv string, origin string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "input.csv")
	_, _ = part.Write([]byte(csv))
	_ = mw.WriteField("email", "")
	_ = mw.WriteField("enrich", "false")
	_ = mw.Close()
	return do(h, "POST", "/api/jobs", buf.Bytes(), map[string]string{"Content-Type": mw.FormDataContentType(), "Origin": origin})
}

func TestUploadReviewFlow(t *testing.T) {
	h := newServer(t)
	w := do(h, "GET", "/api/config", nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mapbox":"pk.test"`) || !strings.Contains(w.Body.String(), `"google":false`) {
		t.Fatalf("config: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("api headers missing")
	}
	w = upload(t, h, "number,name\n0123456749,A\n", "http://example.test")
	if w.Code != 202 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var created struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	var job map[string]any
	for i := 0; i < 50 && job["state"] != "done"; i++ {
		w = do(h, "GET", "/api/jobs/"+created.ID, nil, nil)
		_ = json.Unmarshal(w.Body.Bytes(), &job)
		time.Sleep(10 * time.Millisecond)
	}
	if job["state"] != "done" {
		t.Fatalf("job never finished: %v", job)
	}
	patch := []byte(`{"record":{"id":"1","name":"Edited","reviewed":true}}`)
	w = do(h, "PATCH", "/api/jobs/"+created.ID, patch, map[string]string{"Content-Type": "application/json", "Origin": "http://example.test"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Edited"`) {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	w = do(h, "PATCH", "/api/jobs/"+created.ID, []byte(`{}`), map[string]string{"Content-Type": "application/json", "Origin": "http://example.test"})
	if w.Code != 400 {
		t.Fatalf("empty update accepted: %d", w.Code)
	}
	w = do(h, "GET", "/api/jobs/nope", nil, nil)
	if w.Code != 404 {
		t.Fatalf("missing job: %d", w.Code)
	}
}

func TestRejectsCrossOriginAndOversizedWrites(t *testing.T) {
	h := newServer(t)
	if w := upload(t, h, "number,name\n0123456749,A\n", "http://evil.test"); w.Code != 403 {
		t.Fatalf("cross-origin accepted: %d", w.Code)
	}
	if w := upload(t, h, "number,name\n0123456749,A\n", ""); w.Code != 403 {
		t.Fatalf("missing origin accepted: %d", w.Code)
	}
	if w := upload(t, h, "name,number\nx,y,z", "http://example.test"); w.Code != 400 {
		t.Fatalf("malformed csv: %d", w.Code)
	}
	w := do(h, "POST", "/api/jobs", nil, map[string]string{"Origin": "http://example.test", "Content-Length": "99999999"})
	if w.Code != 413 {
		t.Fatalf("oversized body: %d", w.Code)
	}
}

func TestStaticSiteAndFallback(t *testing.T) {
	h := newServer(t)
	w := do(h, "GET", "/", nil, nil)
	if w.Code != 200 || w.Body.String() != "<html>app</html>" || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("index: %d %q", w.Code, w.Body.String())
	}
	w = do(h, "GET", "/?job=abc", nil, map[string]string{"Accept-Encoding": "gzip, br"})
	if w.Header().Get("Content-Encoding") != "gzip" || w.Body.String() != "gz-index" {
		t.Fatalf("precompressed index not used: %q", w.Header().Get("Content-Encoding"))
	}
	w = do(h, "GET", "/_app/immutable/chunk.js", nil, map[string]string{"Accept-Encoding": "br"})
	if w.Body.String() != "br-js" || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("immutable asset: %q %q", w.Body.String(), w.Header())
	}
	w = do(h, "GET", "/some/client/route", nil, nil)
	if w.Code != 200 || w.Body.String() != "<html>app</html>" {
		t.Fatalf("spa fallback: %d", w.Code)
	}
	if w = do(h, "GET", "/missing.png", nil, nil); w.Code != 404 {
		t.Fatalf("missing asset served: %d", w.Code)
	}
	if w = do(h, "GET", "/api/unknown", nil, nil); w.Code != 404 {
		t.Fatalf("unknown api route: %d", w.Code)
	}
	if w = do(h, "GET", "/healthz", nil, nil); w.Code != 200 {
		t.Fatalf("healthz: %d", w.Code)
	}
}
