package main

import (
	"context"
	"log"
	"net/http"
	"os"

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
	if _, err := server.BootstrapOwner(ctx, db, os.Stdout); err != nil {
		log.Fatal(err)
	}
	staticDir := os.Getenv("NEUL_STATIC_DIR")
	handler := server.NewRouter(server.Config{DB: db, StaticDir: staticDir, HomeDir: os.Getenv("NEUL_HOME_DIR")})
	addr := os.Getenv("NEUL_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
