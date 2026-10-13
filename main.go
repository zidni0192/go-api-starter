package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type item struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type store struct {
	sync.RWMutex
	next  int64
	items map[int64]item
}

func newStore() *store { return &store{next: 1, items: make(map[int64]item)} }

func (s *store) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", s.collection)
	mux.HandleFunc("/items/", s.resource)
	return mux
}

func (s *store) collection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.RLock()
		items := make([]item, 0, len(s.items))
		for _, v := range s.items {
			items = append(items, v)
		}
		s.RUnlock()
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		s.Lock()
		v := item{ID: s.next, Name: strings.TrimSpace(input.Name)}
		s.next++
		s.items[v.ID] = v
		s.Unlock()
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
	s.Lock()
	defer s.Unlock()
	v, ok := s.items[id]
	if !ok {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, v)
	case http.MethodPut:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		v.Name = strings.TrimSpace(input.Name)
		s.items[id] = v
		writeJSON(w, http.StatusOK, v)
	case http.MethodDelete:
		delete(s.items, id)
		w.WriteHeader(http.StatusNoContent)
	}
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
	addr := ":8080"
	log.Printf("listening on %s", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           newStore().handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
