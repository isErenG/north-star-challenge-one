// Package app wires configuration, storage, services and HTTP together.
package app

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"kbo-review/internal/config"
	"kbo-review/internal/enrich"
	"kbo-review/internal/httpapi"
	"kbo-review/internal/jobs"
	"kbo-review/internal/notify"
	"kbo-review/internal/store"
	"kbo-review/web"
)

// Run starts the service and blocks. `kbo-review -check` instead probes the
// running instance's health endpoint and exits 0 or 1, for container
// healthchecks in images that have no curl.
func Run() {
	cfg := config.FromEnv()
	if len(os.Args) > 1 && os.Args[1] == "-check" {
		os.Exit(check(cfg.Addr))
	}
	st, err := store.NewFile(cfg.DataDir)
	if err != nil {
		log.Fatalf("data directory: %v", err)
	}
	svc, err := jobs.New(st, enrich.New(cfg), notify.New(cfg.ResendKey, cfg.NotifyFrom, cfg.PublicAppURL), cfg.DefaultBudget)
	if err != nil {
		log.Fatalf("loading jobs: %v", err)
	}
	log.Fatal(httpapi.Listen(cfg.Addr, httpapi.New(cfg, svc, web.FS())))
}

func check(addr string) int {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(fmt.Sprintf("http://%s/healthz", net.JoinHostPort(host, port)))
	if err != nil || res.StatusCode != http.StatusOK {
		return 1
	}
	res.Body.Close()
	return 0
}
