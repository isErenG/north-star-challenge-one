// Package config reads the process configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config is everything the service needs from its environment.
type Config struct {
	Addr         string // listen address, e.g. 127.0.0.1:8787
	DataDir      string // directory for job JSON files
	GoogleKey    string // Google Places API key; empty disables enrichment
	ResendKey    string // Resend API key; empty disables email
	NotifyFrom   string // verified sender for completion emails
	PublicAppURL string // optional HTTPS origin used in email links
	MapboxToken  string // public (pk.) Mapbox token exposed to the browser

	OpenAIKey     string   // OpenAI API key; empty disables the judge
	OpenAIModel   string   // chat model used for verdicts and extraction
	OpenAICostEUR float64  // estimated cost per judge call
	GoogleCostEUR float64  // estimated cost per Places text search
	DataBeToken   string   // data.be API token; empty disables that source
	Directories   []string // opt-in scraping sources: goldenpages, trendstop
	DefaultBudget float64  // default per-job enrichment budget in EUR
}

// FromEnv builds a Config from the environment with local-run defaults.
func FromEnv() Config {
	return Config{
		Addr:         env("ADDR", "127.0.0.1:8787"),
		DataDir:      env("DATA_DIR", "data"),
		GoogleKey:    os.Getenv("GOOGLE_MAPS_API_KEY"),
		ResendKey:    os.Getenv("RESEND_API_KEY"),
		NotifyFrom:   os.Getenv("NOTIFY_FROM"),
		PublicAppURL: os.Getenv("PUBLIC_APP_URL"),
		MapboxToken:  publicMapboxToken(os.Getenv("MAPBOX_ACCESS_TOKEN")),

		OpenAIKey:     os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:   env("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAICostEUR: envFloat("OPENAI_COST_EUR", 0.002),
		GoogleCostEUR: envFloat("GOOGLE_COST_EUR", 0.03),
		DataBeToken:   os.Getenv("DATABE_TOKEN"),
		Directories:   list(os.Getenv("ENRICH_DIRECTORIES")),
		DefaultBudget: envFloat("ENRICH_DEFAULT_BUDGET_EUR", 5),
	}
}

func envFloat(key string, fallback float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64); err == nil && v >= 0 {
		return v
	}
	return fallback
}

func list(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// publicMapboxToken returns token only when it is a public (pk.) token.
// A secret (sk.) token is never sent to the client.
func publicMapboxToken(token string) string {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "pk.") {
		return ""
	}
	return token
}
