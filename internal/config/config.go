// Package config reads the process configuration from environment variables.
package config

import (
	"os"
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
	}
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
