package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	DB               *sql.DB
	StaticDir        string
	Clock            func() time.Time
	HomeDir          string
	SetupTokenWriter io.Writer
	SetupTokenTTL    time.Duration
}

func NewRouter(config Config) http.Handler {
	if config.Clock == nil {
		config.Clock = func() time.Time { return time.Now().UTC() }
	}
	if config.HomeDir == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			config.HomeDir = homeDir
		}
	}
	if config.SetupTokenWriter == nil {
		config.SetupTokenWriter = os.Stdout
	}
	config.SetupTokenTTL = effectiveSetupTokenTTL(config.SetupTokenTTL)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/session/local", handleLocalSession(config.DB, config.Clock, config.SetupTokenWriter, config.SetupTokenTTL))
	mux.Handle("POST /api/pair/init", requireOwnerSession(config.DB, handlePairInit(config.DB, config.Clock)))
	mux.HandleFunc("POST /api/pair/claim", handlePairClaim(config.DB, config.Clock))
	mux.Handle("GET /api/pair/poll", requireOwnerSession(config.DB, handlePairPoll(config.DB, config.Clock)))
	mux.Handle("GET /api/dashboard", requireOwnerSession(config.DB, handleDashboard(config.DB, config.Clock)))
	mux.Handle("GET /api/machines", requireOwnerSession(config.DB, handleListMachines(config.DB, config.Clock)))
	mux.Handle("GET /api/machines/{machineId}", requireOwnerSession(config.DB, handleGetMachine(config.DB, config.Clock)))
	mux.Handle("POST /api/machines/{machineId}/repair-drift", requireOwnerSession(config.DB, handleRepairDrift(config.DB, config.Clock)))
	mux.Handle("GET /api/resources", requireOwnerSession(config.DB, handleListResources(config.DB)))
	mux.Handle("POST /api/resources/package", requireOwnerSession(config.DB, handleCreatePackageResource(config.DB, config.Clock)))
	mux.Handle("POST /api/resources/dotfile", requireOwnerSession(config.DB, handleCreateDotfileResource(config.DB, config.Clock, config.HomeDir)))
	mux.Handle("PATCH /api/resources/{resourceId}", requireOwnerSession(config.DB, handlePatchResource(config.DB, config.Clock)))
	mux.Handle("DELETE /api/resources/{resourceId}", requireOwnerSession(config.DB, handleDeleteResource(config.DB)))
	mux.Handle("POST /api/agent/heartbeat", requireMachineToken(config.DB, handleAgentHeartbeat(config.DB, config.Clock)))
	mux.Handle("GET /api/agent/desired-state", requireMachineToken(config.DB, handleAgentDesiredState(config.DB)))
	mux.Handle("GET /api/agent/commands", requireMachineToken(config.DB, handleAgentCommands(config.DB)))
	mux.Handle("POST /api/agent/reconcile-report", requireMachineToken(config.DB, handleAgentReport(config.DB, config.Clock, "reconcile")))
	mux.Handle("POST /api/agent/drift-report", requireMachineToken(config.DB, handleAgentReport(config.DB, config.Clock, "drift")))
	mux.HandleFunc("/", spaHandler(config.StaticDir))
	return mux
}

func spaHandler(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusNotFound, "not_found", "API route was not found.")
			return
		}
		if r.URL.Path == "/ws" || strings.HasPrefix(r.URL.Path, "/ws/") {
			writeJSONError(w, http.StatusNotFound, "not_found", "WebSocket routes are not enabled.")
			return
		}
		if staticDir == "" {
			writeJSONError(w, http.StatusNotFound, "not_found", "Static assets are not configured.")
			return
		}
		assetPath := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(assetPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, assetPath)
			return
		}
		body, err := os.ReadFile(filepath.Join(staticDir, "index.html"))
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "not_found", "Static index was not found.")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			return
		}
	}
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]map[string]string{
		"error": {
			"code":    code,
			"message": message,
		},
	})
}
