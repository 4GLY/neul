package main

import (
	"context"
	"fmt"
	"log"
	"net"
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
	addr := os.Getenv("NEUL_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	publicOrigin, err := publicOriginFromEnv(addr)
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
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

func publicOriginFromEnv(addr string) (string, error) {
	raw := strings.TrimSpace(os.Getenv("NEUL_PUBLIC_ORIGIN"))
	if raw == "" {
		if publicOriginRequiredForAddr(addr) {
			return "", fmt.Errorf("NEUL_PUBLIC_ORIGIN is required when NEUL_ADDR is not loopback: %s", addr)
		}
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

func publicOriginRequiredForAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
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
