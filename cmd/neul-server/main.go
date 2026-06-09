package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
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
	})
	addr := os.Getenv("NEUL_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
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
