package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestItemCRUD(t *testing.T) {
	h := newStore().handler()
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
	newStore().handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(`{"name":" "}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
