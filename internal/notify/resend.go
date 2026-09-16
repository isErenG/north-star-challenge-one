// Package notify sends the one completion email a job may request.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const endpoint = "https://api.resend.com/emails"

// Client sends email through Resend.
type Client struct {
	APIKey    string
	From      string
	PublicURL string
	HTTP      *http.Client
}

// New returns a client. Ready is false until both key and sender are set.
func New(apiKey, from, publicURL string) *Client {
	return &Client{APIKey: apiKey, From: from, PublicURL: publicURL, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Ready reports whether email can be sent.
func (c *Client) Ready() bool { return c != nil && c.APIKey != "" && c.From != "" }

// Send delivers the completion notice for job id to email. The job id is the
// idempotency key, so a retry never sends a second message. Provider
// acceptance is returned as true; it is not proof of inbox delivery.
func (c *Client) Send(email, jobID string, total int) bool {
	text := fmt.Sprintf("Your KBO review is ready. %d records have been organised. Return to your open KBO Review tab to review and download the results.", total)
	if strings.HasPrefix(c.PublicURL, "https://") {
		text += "\n\n" + strings.TrimRight(c.PublicURL, "/") + "/?job=" + jobID
	}
	body, _ := json.Marshal(map[string]any{"from": c.From, "to": []string{email}, "subject": "Your KBO review is ready", "text": text})
	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "kbo-job-"+jobID)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode >= 200 && res.StatusCode < 300
}
