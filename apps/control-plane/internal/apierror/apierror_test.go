package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteEmitsStructuredJSON(t *testing.T) {
	proto := New("employee.name_conflict", http.StatusConflict, "该名称已被使用")
	rec := httptest.NewRecorder()

	if !Write(rec, proto.WithCause(errors.New("pg 23505"))) {
		t.Fatal("expected Write to handle *Error")
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content-type %q", ct)
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != "employee.name_conflict" || body.Message != "该名称已被使用" {
		t.Fatalf("unexpected body %#v", body)
	}
}

func TestWriteReturnsFalseForPlainError(t *testing.T) {
	rec := httptest.NewRecorder()
	if Write(rec, errors.New("boom")) {
		t.Fatal("expected Write to decline non-coded error")
	}
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("expected untouched recorder, got code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestIsMatchesByCodeAcrossClonesAndWrapping(t *testing.T) {
	proto := New("employee.avatar_in_use", http.StatusConflict, "该头像已被使用")

	if !errors.Is(proto.WithCause(errors.New("pg")), proto) {
		t.Fatal("WithCause clone should match prototype by code")
	}
	wrapped := fmt.Errorf("create: %w", proto.WithCause(errors.New("pg")))
	if !errors.Is(wrapped, proto) {
		t.Fatal("wrapped coded error should match prototype by code")
	}
	other := New("employee.name_conflict", http.StatusConflict, "x")
	if errors.Is(proto, other) {
		t.Fatal("different codes must not match")
	}
}
