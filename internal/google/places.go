// Package google looks up unconfirmed Places candidates for a record.
// Candidates are suggestions for a human to check; nothing is applied
// automatically.
package google

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"kbo-review/internal/model"
)

const endpoint = "https://places.googleapis.com/v1/places:searchText"
const fieldMask = "places.id,places.displayName,places.formattedAddress,places.businessStatus,places.internationalPhoneNumber,places.websiteUri,places.googleMapsUri"

// Client calls the Places Text Search API.
type Client struct {
	Key  string
	HTTP *http.Client
}

// New returns a client; an empty key means Ready reports false.
func New(key string) *Client {
	return &Client{Key: key, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Ready reports whether a key is configured.
func (c *Client) Ready() bool { return c != nil && c.Key != "" }

// Lookup returns the top candidate for the record, or a human-readable reason
// why no candidate was confirmed. One billable call per record.
func (c *Client) Lookup(row model.Record) (*model.Place, string) {
	if row.Name == "" || row.Address == "" {
		return nil, "Missing name or address; lookup skipped"
	}
	body, _ := json.Marshal(map[string]any{"textQuery": row.Name + " " + row.Address + " Belgium", "regionCode": "BE", "pageSize": 1})
	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.Key)
	req.Header.Set("X-Goog-FieldMask", fieldMask)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "Google Maps could not be reached"
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Sprintf("Google Maps returned %d; no match confirmed", res.StatusCode)
	}
	var payload struct {
		Places []model.Place `json:"places"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload) != nil {
		return nil, "Google Maps returned an unreadable response"
	}
	if len(payload.Places) == 0 {
		return nil, "No candidate found"
	}
	return &payload.Places[0], ""
}
