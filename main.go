package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type item struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type store struct {
	db *sql.DB
}

func openStore(ctx context.Context, path string) (*store, error) {
	if path == "" {
		path = "data.db"
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &store{db: db}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	var applied bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 1)").Scan(&applied); err != nil {
		return fmt.Errorf("check migration version: %w", err)
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL CHECK(length(trim(name)) > 0)
		)`); err != nil {
			return fmt.Errorf("create items table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (1)"); err != nil {
			return fmt.Errorf("record migration: %w", err)
		}
	}
	return tx.Commit()
}

func (s *store) close() error { return s.db.Close() }

func (s *store) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", s.collection)
	mux.HandleFunc("/items/", s.resource)
	return mux
}

func (s *store) collection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.QueryContext(r.Context(), "SELECT id, name FROM items ORDER BY id")
		if err != nil {
			http.Error(w, "could not list items", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		items := make([]item, 0)
		for rows.Next() {
			var v item
			if err := rows.Scan(&v.ID, &v.Name); err != nil {
				http.Error(w, "could not read items", http.StatusInternalServerError)
				return
			}
			items = append(items, v)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "could not read items", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		v := item{Name: strings.TrimSpace(input.Name)}
		result, err := s.db.ExecContext(r.Context(), "INSERT INTO items(name) VALUES (?)", v.Name)
		if err != nil {
			http.Error(w, "could not create item", http.StatusInternalServerError)
			return
		}
		v.ID, err = result.LastInsertId()
		if err != nil {
			http.Error(w, "could not create item", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *store) resource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodGet, http.MethodPut, http.MethodDelete)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/items/"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "invalid item id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := s.get(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "item not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "could not get item", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, v)
	case http.MethodPut:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(input.Name)
		result, err := s.db.ExecContext(r.Context(), "UPDATE items SET name = ? WHERE id = ?", name, id)
		if err != nil {
			http.Error(w, "could not update item", http.StatusInternalServerError)
			return
		}
		affected, err := result.RowsAffected()
		if err != nil {
			http.Error(w, "could not update item", http.StatusInternalServerError)
			return
		}
		if affected == 0 {
			http.Error(w, "item not found", http.StatusNotFound)
			return
		}
		v := item{ID: id, Name: name}
		writeJSON(w, http.StatusOK, v)
	case http.MethodDelete:
		result, err := s.db.ExecContext(r.Context(), "DELETE FROM items WHERE id = ?", id)
		if err != nil {
			http.Error(w, "could not delete item", http.StatusInternalServerError)
			return
		}
		affected, err := result.RowsAffected()
		if err != nil {
			http.Error(w, "could not delete item", http.StatusInternalServerError)
			return
		}
		if affected == 0 {
			http.Error(w, "item not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *store) get(ctx context.Context, id int64) (item, error) {
	var v item
	err := s.db.QueryRowContext(ctx, "SELECT id, name FROM items WHERE id = ?", id).Scan(&v.ID, &v.Name)
	return v, err
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func main() {
	path := os.Getenv("DATABASE_PATH")
	ctx := context.Background()
	items, err := openStore(ctx, path)
	if err != nil {
		log.Fatal(err)
	}
	defer items.close()
	addr := ":8080"
	log.Printf("listening on %s", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           items.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
