// Package app owns the demo application's HTTP and PostgreSQL boundary.
// Migrations and schema verification run before traffic is served so an
// incompatible schema produces an observable Kubernetes restart.
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/pressly/goose/v3"
)

const migrationTable = "goose_db_version"

type App struct {
	db     *sql.DB
	logger *slog.Logger
	mux    *http.ServeMux
}

type message struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// New migrates the database and returns a ready HTTP application.
func New(ctx context.Context, db *sql.DB, logger *slog.Logger, migrationsDir string) (*App, error) {
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := runMigrations(ctx, db, migrationsDir); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if err := verifySchema(ctx, db); err != nil {
		return nil, fmt.Errorf("verify application schema: %w", err)
	}

	a := &App{db: db, logger: logger, mux: http.NewServeMux()}
	a.routes()
	return a, nil
}

func runMigrations(ctx context.Context, db *sql.DB, migrationsDir string) error {
	goose.SetTableName(migrationTable)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, migrationsDir)
}

// verifySchema exercises the same column contract as the request path. An
// incompatible migration must fail startup so Kubernetes records the incident.
func verifySchema(ctx context.Context, db *sql.DB) error {
	var count int64
	return db.QueryRowContext(ctx, `SELECT count(body) FROM messages`).Scan(&count)
}

func (a *App) routes() {
	a.mux.HandleFunc("GET /", a.home)
	a.mux.HandleFunc("GET /healthz", a.health)
	a.mux.HandleFunc("GET /readyz", a.ready)
	a.mux.HandleFunc("GET /api/messages", a.listMessages)
	a.mux.HandleFunc("POST /api/messages", a.createMessage)
}

// Handler returns the application's HTTP handler.
func (a *App) Handler() http.Handler {
	return a.logging(a.mux)
}

func (a *App) home(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"application": "mission-control demo",
		"endpoints":   []string{"GET /api/messages", "POST /api/messages", "GET /healthz", "GET /readyz"},
	})
}

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *App) listMessages(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT id, body, created_at
		FROM messages
		ORDER BY id DESC
		LIMIT 100
	`)
	if err != nil {
		a.internalError(w, "query messages", err)
		return
	}
	defer rows.Close()

	messages := make([]message, 0)
	for rows.Next() {
		var item message
		if err := rows.Scan(&item.ID, &item.Text, &item.CreatedAt); err != nil {
			a.internalError(w, "scan message", err)
			return
		}
		messages = append(messages, item)
	}
	if err := rows.Err(); err != nil {
		a.internalError(w, "iterate messages", err)
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

func (a *App) createMessage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Text string `json:"text"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if input.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	var item message
	err := a.db.QueryRowContext(r.Context(), `
		INSERT INTO messages (body)
		VALUES ($1)
		RETURNING id, body, created_at
	`, input.Text).Scan(&item.ID, &item.Text, &item.CreatedAt)
	if err != nil {
		a.internalError(w, "insert message", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (a *App) internalError(w http.ResponseWriter, operation string, err error) {
	a.logger.Error(operation, "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func (a *App) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		a.logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		slog.Error("encode response", "error", err)
	}
}
