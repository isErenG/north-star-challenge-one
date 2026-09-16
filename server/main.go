package main

import (
	"bytes"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Record struct {
	ID           string         `json:"id"`
	Number       string         `json:"number"`
	Enterprise   string         `json:"enterprise"`
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	Address      string         `json:"address"`
	Municipality string         `json:"municipality"`
	Status       string         `json:"status"`
	Phone        string         `json:"phone"`
	Email        string         `json:"email"`
	Website      string         `json:"website"`
	Notes        string         `json:"notes"`
	Reviewed     bool           `json:"reviewed"`
	Issues       []string       `json:"issues"`
	Geometry     any            `json:"geometry"`
	Source       map[string]any `json:"source"`
	Google       *Place         `json:"google,omitempty"`
	GoogleError  string         `json:"googleError,omitempty"`
}
type Place struct {
	ID          string `json:"id"`
	DisplayName struct {
		Text string `json:"text"`
	} `json:"displayName"`
	FormattedAddress string `json:"formattedAddress"`
	BusinessStatus   string `json:"businessStatus"`
	Phone            string `json:"internationalPhoneNumber"`
	Website          string `json:"websiteUri"`
	MapsURL          string `json:"googleMapsUri"`
}
type Job struct {
	ID           string   `json:"id"`
	Filename     string   `json:"filename"`
	Created      string   `json:"created"`
	State        string   `json:"state"`
	Phase        string   `json:"phase"`
	Progress     int      `json:"progress"`
	Total        int      `json:"total"`
	Records      []Record `json:"records"`
	Email        string   `json:"email"`
	Notification string   `json:"notification"`
	Enrich       bool     `json:"enrich"`
	Error        string   `json:"error,omitempty"`
}
type Store struct {
	sync.Mutex
	jobs  map[string]*Job
	dir   string
	slots chan struct{}
}

var db = Store{jobs: map[string]*Job{}, slots: make(chan struct{}, 2)}
var client = &http.Client{Timeout: 15 * time.Second}

func main() {
	db.dir = os.Getenv("DATA_DIR")
	if db.dir == "" {
		db.dir = "data"
	}
	if err := os.MkdirAll(db.dir, 0700); err != nil {
		log.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(db.dir, "*.json"))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var j Job
		if json.Unmarshal(b, &j) != nil {
			continue
		}
		if j.State == "processing" {
			j.State = "failed"
			j.Error = "Processing was interrupted. Please upload your file again."
		}
		if j.Notification == "sending" {
			j.Notification = "delivery unknown"
		}
		db.jobs[j.ID] = &j
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"google": os.Getenv("GOOGLE_MAPS_API_KEY") != "", "email": emailReady()})
	})
	mux.HandleFunc("POST /api/jobs", createJob)
	mux.HandleFunc("GET /api/jobs/{id}", getJob)
	mux.HandleFunc("PATCH /api/jobs/{id}", updateJob)
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8787"
	}
	log.Printf("KBO review API listening on 127.0.0.1:%s", port)
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
func respond(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) {
	respond(w, code, map[string]string{"error": msg})
}
func emailReady() bool { return os.Getenv("RESEND_API_KEY") != "" && os.Getenv("NOTIFY_FROM") != "" }
func validEmail(s string) bool {
	a, e := mail.ParseAddress(s)
	return e == nil && a.Address == s && len(s) <= 254
}
func (s *Store) save(j *Job) error {
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	p := filepath.Join(s.dir, j.ID+".json")
	if e = os.WriteFile(p+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}
func createJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		fail(w, 400, "Upload a file smaller than 10 MB.")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "Choose a CSV, JSON or GeoJSON file.")
		return
	}
	defer f.Close()
	email := strings.TrimSpace(r.FormValue("email"))
	if email != "" && (!emailReady() || !validEmail(email)) {
		fail(w, 400, "A valid email and a configured notification service are required.")
		return
	}
	enrich := r.FormValue("enrich") == "true"
	if enrich && os.Getenv("GOOGLE_MAPS_API_KEY") == "" {
		fail(w, 400, "Google Maps is not connected.")
		return
	}
	records, err := parse(f, strings.ToLower(filepath.Ext(h.Filename)))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	select {
	case db.slots <- struct{}{}:
	default:
		fail(w, 429, "Two files are already processing. Please try again shortly.")
		return
	}
	id := make([]byte, 24)
	if _, err = rand.Read(id); err != nil {
		<-db.slots
		fail(w, 500, "Could not create a job.")
		return
	}
	j := &Job{ID: hex.EncodeToString(id), Filename: filepath.Base(h.Filename), Created: time.Now().UTC().Format(time.RFC3339), State: "processing", Phase: "structure", Total: len(records), Records: records, Email: email, Enrich: enrich, Notification: "not requested"}
	if email != "" {
		j.Notification = "pending"
	}
	db.Lock()
	db.jobs[j.ID] = j
	err = db.save(j)
	if err != nil {
		delete(db.jobs, j.ID)
	}
	db.Unlock()
	if err != nil {
		<-db.slots
		fail(w, 500, "Could not save the upload.")
		return
	}
	respond(w, 202, map[string]string{"id": j.ID})
	go process(j)
}
func getJob(w http.ResponseWriter, r *http.Request) {
	db.Lock()
	defer db.Unlock()
	j, ok := db.jobs[r.PathValue("id")]
	if !ok {
		fail(w, 404, "This task was not found. Upload your file to start again.")
		return
	}
	respond(w, 200, j)
}
func updateJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var input struct {
		Email  *string `json:"email"`
		Record *struct {
			ID, Name, Address, Phone, Email, Website, Notes string
			Reviewed                                        bool
		} `json:"record"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		fail(w, 400, "Invalid update.")
		return
	}
	if (input.Email == nil) == (input.Record == nil) {
		fail(w, 400, "Send either a record edit or an email update.")
		return
	}
	db.Lock()
	j, ok := db.jobs[r.PathValue("id")]
	if !ok {
		db.Unlock()
		fail(w, 404, "Task not found.")
		return
	}
	before, _ := json.Marshal(j)
	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if !emailReady() || !validEmail(email) {
			db.Unlock()
			fail(w, 400, "Enter a valid email. Notifications must be configured.")
			return
		}
		if j.Notification == "sent" || j.Notification == "sending" {
			db.Unlock()
			fail(w, 409, "A completion email has already been queued.")
			return
		}
		j.Email = email
		j.Notification = "pending"
	}
	if input.Record != nil {
		if j.State != "done" {
			db.Unlock()
			fail(w, 409, "Wait until processing finishes.")
			return
		}
		x := input.Record
		found := false
		for i := range j.Records {
			row := &j.Records[i]
			if row.ID == x.ID {
				row.Name = strings.TrimSpace(x.Name)
				row.Address = strings.TrimSpace(x.Address)
				row.Phone = strings.TrimSpace(x.Phone)
				row.Email = strings.TrimSpace(x.Email)
				row.Website = strings.TrimSpace(x.Website)
				row.Notes = x.Notes
				row.Reviewed = x.Reviewed
				validate(row)
				found = true
				break
			}
		}
		if !found {
			db.Unlock()
			fail(w, 404, "Record not found.")
			return
		}
	}
	if err := db.save(j); err != nil {
		_ = json.Unmarshal(before, j)
		db.Unlock()
		fail(w, 500, "Your changes could not be saved. Please try again.")
		return
	}
	notify := j.State == "done" && j.Notification == "pending"
	respond(w, 200, j)
	db.Unlock()
	if notify {
		go sendNotification(j)
	}
}
func process(j *Job) {
	defer func() { <-db.slots }()
	for i := 0; i < j.Total; i++ {
		db.Lock()
		row := j.Records[i]
		j.Phase = "validation"
		db.Unlock()
		validate(&row)
		if j.Enrich {
			db.Lock()
			j.Phase = "google"
			db.Unlock()
			row.Google, row.GoogleError = lookup(row)
		}
		db.Lock()
		j.Records[i] = row
		j.Progress = i + 1
		db.Unlock()
	}
	db.Lock()
	counts := map[string]int{}
	for _, row := range j.Records {
		if row.Number != "" {
			counts[row.Number]++
		}
	}
	for i := range j.Records {
		if counts[j.Records[i].Number] > 1 {
			j.Records[i].Issues = append(j.Records[i].Issues, "Duplicate identifier")
		}
	}
	j.Phase = "complete"
	j.State = "done"
	if err := db.save(j); err != nil {
		j.State = "failed"
		j.Error = "Results could not be saved. Please try again."
	}
	notify := j.State == "done" && j.Email != ""
	db.Unlock()
	if notify {
		sendNotification(j)
	}
}
func sendNotification(j *Job) {
	db.Lock()
	if j.Notification != "pending" {
		db.Unlock()
		return
	}
	j.Notification = "sending"
	email := j.Email
	total := j.Total
	id := j.ID
	if db.save(j) != nil {
		j.Notification = "failed"
		db.Unlock()
		return
	}
	db.Unlock()
	text := fmt.Sprintf("Your KBO review is ready. %d records have been organised. Return to your open KBO Review tab to review and download the results.", total)
	if base := os.Getenv("PUBLIC_APP_URL"); strings.HasPrefix(base, "https://") {
		text += "\n\n" + strings.TrimRight(base, "/") + "/?job=" + id
	}
	body, _ := json.Marshal(map[string]any{"from": os.Getenv("NOTIFY_FROM"), "to": []string{email}, "subject": "Your KBO review is ready", "text": text})
	req, _ := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+os.Getenv("RESEND_API_KEY"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "kbo-job-"+id)
	res, err := client.Do(req)
	status := "failed"
	if err == nil {
		defer res.Body.Close()
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			status = "sent"
		}
	}
	db.Lock()
	j.Notification = status
	_ = db.save(j)
	db.Unlock()
}
func lookup(row Record) (*Place, string) {
	if row.Name == "" || row.Address == "" {
		return nil, "Missing name or address; lookup skipped"
	}
	body, _ := json.Marshal(map[string]any{"textQuery": row.Name + " " + row.Address + " Belgium", "regionCode": "BE", "pageSize": 1})
	req, _ := http.NewRequest("POST", "https://places.googleapis.com/v1/places:searchText", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", os.Getenv("GOOGLE_MAPS_API_KEY"))
	req.Header.Set("X-Goog-FieldMask", "places.id,places.displayName,places.formattedAddress,places.businessStatus,places.internationalPhoneNumber,places.websiteUri,places.googleMapsUri")
	res, err := client.Do(req)
	if err != nil {
		return nil, "Google Maps could not be reached"
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Sprintf("Google Maps returned %d; no match confirmed", res.StatusCode)
	}
	var payload struct {
		Places []Place `json:"places"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload) != nil {
		return nil, "Google Maps returned an unreadable response"
	}
	if len(payload.Places) == 0 {
		return nil, "No candidate found"
	}
	return &payload.Places[0], ""
}
func field(m map[string]any, keys ...string) string {
	for _, key := range keys {
		for k, v := range m {
			if strings.EqualFold(k, key) && v != nil {
				s := strings.TrimSpace(fmt.Sprint(v))
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}
func parse(r io.Reader, ext string) ([]Record, error) {
	var raw []map[string]any
	var geometries []any
	switch ext {
	case ".csv":
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, errors.New("Could not read this CSV")
		}
		text := strings.TrimPrefix(string(b), "\ufeff")
		reader := csv.NewReader(strings.NewReader(text))
		line := strings.SplitN(text, "\n", 2)[0]
		if strings.Count(line, ";") > strings.Count(line, ",") {
			reader.Comma = ';'
		}
		if strings.Count(line, "\t") > strings.Count(line, string(reader.Comma)) {
			reader.Comma = '\t'
		}
		headers, err := reader.Read()
		if err != nil {
			return nil, errors.New("The CSV needs a header row")
		}
		seen := map[string]bool{}
		for i, h := range headers {
			h = strings.TrimSpace(h)
			if h == "" || seen[h] {
				return nil, errors.New("CSV column names must be non-empty and unique")
			}
			headers[i] = h
			seen[h] = true
		}
		for {
			values, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("CSV row %d is malformed: check its column count and quotes", len(raw)+2)
			}
			m := map[string]any{}
			for i, h := range headers {
				m[h] = values[i]
			}
			raw = append(raw, m)
			if len(raw) > 10000 {
				return nil, errors.New("Upload up to 10,000 records per file")
			}
		}
	case ".json", ".geojson":
		decoder := json.NewDecoder(r)
		decoder.UseNumber()
		var data any
		if decoder.Decode(&data) != nil {
			return nil, errors.New("This file is not valid JSON")
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			return nil, errors.New("Unexpected content after the JSON document")
		}
		var rows []any
		if list, ok := data.([]any); ok {
			rows = list
		} else if obj, ok := data.(map[string]any); ok {
			if obj["type"] == "FeatureCollection" {
				var ok bool
				rows, ok = obj["features"].([]any)
				if !ok {
					return nil, errors.New("GeoJSON must contain a features array")
				}
				for _, v := range rows {
					f, ok := v.(map[string]any)
					if !ok || f["type"] != "Feature" {
						return nil, errors.New("GeoJSON contains an invalid feature")
					}
					p, ok := f["properties"].(map[string]any)
					if !ok {
						return nil, errors.New("Each feature needs properties")
					}
					raw = append(raw, p)
					geometries = append(geometries, f["geometry"])
				}
				rows = nil
			} else {
				rows, _ = obj["records"].([]any)
				if rows == nil {
					rows, _ = obj["data"].([]any)
				}
				if rows == nil {
					return nil, errors.New("JSON must be an array, a records object, or a GeoJSON FeatureCollection")
				}
			}
		} else {
			return nil, errors.New("JSON must contain records")
		}
		for _, v := range rows {
			m, ok := v.(map[string]any)
			if !ok {
				return nil, errors.New("Each record must be a JSON object")
			}
			raw = append(raw, m)
		}
	default:
		return nil, errors.New("Choose a .csv, .json or .geojson file")
	}
	if len(raw) == 0 {
		return nil, errors.New("The file has no records")
	}
	if len(raw) > 10000 {
		return nil, errors.New("Upload up to 10,000 records per file")
	}
	records := make([]Record, 0, len(raw))
	recognized := false
	for i, m := range raw {
		source := m
		if original, ok := m["source"].(map[string]any); ok {
			source = original
		}
		row := Record{ID: strconv.Itoa(i + 1), Number: field(m, "number", "Ondernemingsnr", "EnterpriseNumber", "enterprise_number", "establishmentNumber"), Enterprise: field(m, "enterprise", "Ondernemingsnr_maatsch_zetel"), Name: field(m, "name", "Maatschappelijke_naam", "Commerciele_naam", "Denomination", "business_name"), Address: field(m, "address", "formattedAddress"), Municipality: field(m, "municipality", "KBO_Gemeente", "city"), Status: field(m, "status", "Rechtstoestand"), Phone: field(m, "phone", "telephone", "phone_number"), Email: field(m, "email", "email_address"), Website: field(m, "website", "url"), Notes: field(m, "notes"), Source: source, Issues: []string{}}
		row.Reviewed, _ = m["reviewed"].(bool)
		if row.Address == "" {
			street := strings.TrimSpace(field(m, "KBO_Straat", "street") + " " + field(m, "KBO_Huisnr", "houseNumber"))
			if box := field(m, "KBO_Busnr", "box"); box != "" {
				street += " bus " + box
			}
			locality := strings.TrimSpace(field(m, "KBO_Postcode", "postcode", "postalCode") + " " + row.Municipality)
			row.Address = strings.Trim(strings.Join([]string{street, locality}, ", "), ", ")
		}
		n := digits(row.Number)
		row.Kind = "enterprise"
		if len(n) == 10 && n[0] >= '2' {
			row.Kind = "establishment"
		}
		if row.Enterprise == "" && row.Kind == "enterprise" {
			row.Enterprise = row.Number
		}
		if i < len(geometries) {
			row.Geometry = geometries[i]
		} else if g, ok := m["geometry"]; ok {
			row.Geometry = g
		} else {
			lon, e1 := strconv.ParseFloat(field(m, "longitude", "lon", "lng"), 64)
			lat, e2 := strconv.ParseFloat(field(m, "latitude", "lat"), 64)
			if e1 == nil && e2 == nil && lon >= -180 && lon <= 180 && lat >= -90 && lat <= 90 {
				row.Geometry = map[string]any{"type": "Point", "coordinates": []float64{lon, lat}}
			}
		}
		if row.Name != "" || row.Number != "" {
			recognized = true
		}
		records = append(records, row)
	}
	if !recognized {
		return nil, errors.New("No business columns found. Include name or Maatschappelijke_naam, and number or Ondernemingsnr. Download the example for a template.")
	}
	return records, nil
}
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func validNumber(s string) bool {
	n := digits(s)
	if len(n) != 10 {
		return false
	}
	a, _ := strconv.Atoi(n[:8])
	b, _ := strconv.Atoi(n[8:])
	return 97-a%97 == b
}
func validate(r *Record) {
	duplicate := false
	for _, issue := range r.Issues {
		if issue == "Duplicate identifier" {
			duplicate = true
		}
	}
	r.Issues = []string{}
	if !validNumber(r.Number) {
		r.Issues = append(r.Issues, "Check identifier")
	}
	if r.Name == "" {
		r.Issues = append(r.Issues, "Missing business name")
	}
	if r.Address == "" {
		r.Issues = append(r.Issues, "Missing address")
	}
	if r.Email != "" && !validEmail(r.Email) {
		r.Issues = append(r.Issues, "Check email format")
	}
	if r.Geometry == nil {
		r.Issues = append(r.Issues, "No coordinates")
	}
	status := strings.ToLower(r.Status)
	if strings.Contains(status, "stop") || strings.Contains(status, "closed") || strings.Contains(status, "ontbind") || strings.Contains(status, "cess") {
		r.Issues = append(r.Issues, "Check registered status")
	}
	if duplicate {
		r.Issues = append(r.Issues, "Duplicate identifier")
	}
}
