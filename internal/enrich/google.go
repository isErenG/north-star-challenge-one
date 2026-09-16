package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kbo-review/internal/model"
)

const placesEndpoint = "https://places.googleapis.com/v1/places:searchText"
const placesFieldMask = "places.id,places.displayName,places.formattedAddress,places.businessStatus,places.internationalPhoneNumber,places.nationalPhoneNumber,places.websiteUri,places.googleMapsUri,places.types,places.primaryType,places.primaryTypeDisplayName,places.location"

// Google runs one Places Text Search per record. Billable.
type Google struct {
	Key     string
	Cost    float64
	HTTP    *http.Client
	Limiter *Limiter
	Cache   Cache
}

func NewGoogle(key string, costEUR float64, cache Cache) *Google {
	return &Google{Key: key, Cost: costEUR, HTTP: &http.Client{Timeout: 15 * time.Second}, Limiter: NewLimiter(10), Cache: cache}
}

func (g *Google) Name() string     { return "google" }
func (g *Google) Ready() bool      { return g != nil && g.Key != "" }
func (g *Google) CostEUR() float64 { return g.Cost }

type place struct {
	model.Place
	NationalPhone string `json:"nationalPhoneNumber"`
}

type placesResponse struct {
	Places []place `json:"places"`
	Status int     `json:"-"`
	Err    string  `json:"-"`
}

func (g *Google) Run(ctx context.Context, row *Row) error {
	if row.Record.Name == "" {
		row.Signals = append(row.Signals, Signal{Source: g.Name(), Existence: Unknown, Note: "No name to search for"})
		return nil
	}
	query := strings.TrimSpace(SearchName(row.Record) + " " + row.Record.Address)
	var res placesResponse
	if !g.Cache.Get("google", query, &res) {
		if err := g.Limiter.Wait(ctx, "google"); err != nil {
			return err
		}
		res = g.search(ctx, query)
		if res.Err == "" {
			g.Cache.Put("google", query, res)
		}
	}
	g.interpret(row, res)
	return nil
}

func (g *Google) search(ctx context.Context, query string) placesResponse {
	body, _ := json.Marshal(map[string]any{"textQuery": query + " Belgium", "regionCode": "BE", "pageSize": 1})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, placesEndpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", g.Key)
	req.Header.Set("X-Goog-FieldMask", placesFieldMask)
	res, err := g.HTTP.Do(req)
	if err != nil {
		return placesResponse{Err: "Google Maps could not be reached"}
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return placesResponse{Status: res.StatusCode, Err: fmt.Sprintf("Google Maps returned %d", res.StatusCode)}
	}
	var out placesResponse
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out) != nil {
		return placesResponse{Err: "Google Maps returned an unreadable response"}
	}
	return out
}

func (g *Google) interpret(row *Row, res placesResponse) {
	src := g.Name()
	if res.Err != "" {
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Unknown, Note: res.Err})
		return
	}
	if len(res.Places) == 0 {
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Unknown, Note: "No Google Maps listing found"})
		return
	}
	p := res.Places[0]
	if isAddressOnly(p) {
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Unknown, Note: "No Google Maps business listing found (only the street address)"})
		return
	}
	copyPlace := p.Place
	row.Place = &copyPlace
	sim := BestSimilarity(row.Record, p.DisplayName.Text)
	scale := 1.0
	mismatch := ""
	if sim < 0.5 {
		scale = 0.4
		mismatch = fmt.Sprintf(" (listing name “%s” differs from the registered name)", p.DisplayName.Text)
	}
	url := p.MapsURL
	switch p.BusinessStatus {
	case "OPERATIONAL":
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Alive, Weight: 0.7 * scale, Note: "Google Maps lists it as operational" + mismatch, URL: url})
	case "CLOSED_PERMANENTLY":
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Closed, Weight: 0.9 * scale, Note: "Google Maps marks it permanently closed" + mismatch, URL: url})
	case "CLOSED_TEMPORARILY":
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Alive, Weight: 0.3 * scale, Note: "Google Maps marks it temporarily closed" + mismatch, URL: url})
	default:
		row.Signals = append(row.Signals, Signal{Source: src, Existence: Unknown, Note: "Google Maps listing has no business status" + mismatch, URL: url})
	}
	conf := 0.7 * scale
	note := "Google Maps listing" + mismatch
	if phone := firstNonEmpty(p.Phone, p.NationalPhone); phone != "" {
		row.Findings = append(row.Findings, Finding{Source: src, Field: "phone", Value: phone, URL: url, Note: note, Confidence: conf})
	}
	if p.Website != "" {
		row.Findings = append(row.Findings, Finding{Source: src, Field: "website", Value: p.Website, URL: url, Note: note, Confidence: conf})
	}
	if p.FormattedAddress != "" {
		row.Findings = append(row.Findings, Finding{Source: src, Field: "address", Value: strings.TrimSuffix(p.FormattedAddress, ", Belgium"), URL: url, Note: note, Confidence: conf})
	}
	if sim >= 0.5 && p.DisplayName.Text != "" {
		row.Findings = append(row.Findings, Finding{Source: src, Field: "name", Value: p.DisplayName.Text, URL: url, Note: note, Confidence: 0.5})
	}
	if industry := p.Industry(); industry != "" {
		row.Findings = append(row.Findings, Finding{Source: src, Field: "activity", Value: industry, URL: url, Note: "Google Maps category" + mismatch, Confidence: 0.5 * scale})
	}
	if len(p.Types) > 0 || p.Industry() != "" {
		row.Pages = append(row.Pages, Page{URL: url, Title: "Google Maps listing", Text: "Google primary category: " + p.Industry() + ". Google place types: " + strings.Join(p.Types, ", ")})
	}
}

// isAddressOnly reports whether Google returned a geocoded address rather
// than a business. Those results carry no evidence about the business.
func isAddressOnly(p place) bool {
	if len(p.Types) == 0 {
		return p.BusinessStatus == "" && p.Phone == "" && p.Website == ""
	}
	for _, t := range p.Types {
		switch t {
		case "street_address", "premise", "subpremise", "route", "geocode", "postal_code", "locality", "plus_code", "political":
			continue
		default:
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
