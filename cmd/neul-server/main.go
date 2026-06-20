package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/4gly/neul/internal/server"
	"github.com/4gly/neul/internal/store"
)

func main() {
	ctx := context.Background()
	dbPath := os.Getenv("NEUL_DB")
	if dbPath == "" {
		dbPath = "neul.sqlite"
	}
	db, err := store.OpenSQLite(ctx, dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := store.ApplyMigrations(ctx, db); err != nil {
		log.Fatal(err)
	}
	setupTokenTTL, err := setupTokenTTLFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	publicOrigin, err := publicOriginFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := server.BootstrapOwnerWithTTL(ctx, db, os.Stdout, setupTokenTTL); err != nil {
		log.Fatal(err)
	}
	staticDir := os.Getenv("NEUL_STATIC_DIR")
	handler := server.NewRouter(server.Config{
		DB:               db,
		StaticDir:        staticDir,
		HomeDir:          os.Getenv("NEUL_HOME_DIR"),
		SetupTokenWriter: os.Stdout,
		SetupTokenTTL:    setupTokenTTL,
		PublicOrigin:     publicOrigin,
	})
	addr := os.Getenv("NEUL_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

func publicOriginFromEnv() (string, error) {
	raw := strings.TrimSpace(os.Getenv("NEUL_PUBLIC_ORIGIN"))
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("NEUL_PUBLIC_ORIGIN must be an origin URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("NEUL_PUBLIC_ORIGIN scheme must be http or https: %s", raw)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("NEUL_PUBLIC_ORIGIN must include only scheme and host: %s", raw)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func setupTokenTTLFromEnv() (time.Duration, error) {
	raw := os.Getenv("NEUL_SETUP_TOKEN_TTL")
	if raw == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("NEUL_SETUP_TOKEN_TTL must be positive: %s", raw)
	}
	return duration, nil
}
