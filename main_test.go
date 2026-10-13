package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestItemCRUD(t *testing.T) {
	s := testStore(t, filepath.Join(t.TempDir(), "items.db"))
	h := s.handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	if got := request(http.MethodPost, "/items", `{"name":"  first  "}`).Code; got != http.StatusCreated {
		t.Fatalf("create status = %d", got)
	}
	list := request(http.MethodGet, "/items", "")
	var items []item
	if err := json.Unmarshal(list.Body.Bytes(), &items); err != nil || len(items) != 1 || items[0].Name != "first" {
		t.Fatalf("list = %s, err = %v", list.Body, err)
	}
	if got := request(http.MethodPut, "/items/1", `{"name":"updated"}`).Code; got != http.StatusOK {
		t.Fatalf("update status = %d", got)
	}
	if got := request(http.MethodDelete, "/items/1", "").Code; got != http.StatusNoContent {
		t.Fatalf("delete status = %d", got)
	}
	if got := request(http.MethodGet, "/items/1", "").Code; got != http.StatusNotFound {
		t.Fatalf("get deleted status = %d", got)
	}
}

func TestCreateRejectsEmptyName(t *testing.T) {
	w := httptest.NewRecorder()
	s := testStore(t, filepath.Join(t.TempDir(), "items.db"))
	s.handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(`{"name":" "}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStorePersistsAcrossReopenAndMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "items.db")
	s := testStore(t, path)
	response := httptest.NewRecorder()
	s.handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(`{"name":"saved"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body)
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := openStore(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { reopened.close() })
	// Re-running startup migration must retain existing rows.
	if err := migrate(context.Background(), reopened.db); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}
	response = httptest.NewRecorder()
	reopened.handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/items", nil))
	var items []item
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil || len(items) != 1 || items[0].Name != "saved" {
		t.Fatalf("persisted list = %s, err = %v", response.Body, err)
	}
}

func TestMissingIDsReturnNotFound(t *testing.T) {
	s := testStore(t, filepath.Join(t.TempDir(), "items.db"))
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, "/items/404", ""},
		{http.MethodPut, "/items/404", `{"name":"absent"}`},
		{http.MethodDelete, "/items/404", ""},
	} {
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, httptest.NewRequest(request.method, request.path, bytes.NewBufferString(request.body)))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s status = %d, want %d", request.method, request.path, w.Code, http.StatusNotFound)
		}
	}
}

func testStore(t *testing.T, path string) *store {
	t.Helper()
	s, err := openStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.close() })
	return s
}
