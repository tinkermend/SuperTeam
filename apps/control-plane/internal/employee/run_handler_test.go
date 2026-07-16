package employee

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TestParseRunListFilterRunKind asserts that the run_kind query parameter is
// parsed into DigitalEmployeeRunListFilter.RunKind: absent/empty means "no
// filter" (nil), a valid value (task/chat) is forwarded as a pointer to that
// exact value, and any other value is rejected with the same message
// ErrInvalidRunKind carries so writeHandlerError-style 400 mapping stays
// consistent with CreateRun's run_kind validation.
func TestParseRunListFilterRunKind(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		wantRunKind *string
		wantErr     string
	}{
		{name: "absent", query: "", wantRunKind: nil, wantErr: ""},
		{name: "task", query: "run_kind=task", wantRunKind: stringPtr("task"), wantErr: ""},
		{name: "chat", query: "run_kind=chat", wantRunKind: stringPtr("chat"), wantErr: ""},
		{name: "invalid", query: "run_kind=banana", wantRunKind: nil, wantErr: ErrInvalidRunKind.Error()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/runs?"+tc.query, nil)
			filter, parseErr := parseRunListFilter(r)
			if parseErr != tc.wantErr {
				t.Fatalf("expected parse error %q, got %q", tc.wantErr, parseErr)
			}
			if tc.wantErr != "" {
				if filter.RunKind != nil {
					t.Fatalf("expected nil RunKind on error, got %q", *filter.RunKind)
				}
				return
			}
			if tc.wantRunKind == nil {
				if filter.RunKind != nil {
					t.Fatalf("expected nil RunKind, got %q", *filter.RunKind)
				}
				return
			}
			if filter.RunKind == nil || *filter.RunKind != *tc.wantRunKind {
				t.Fatalf("expected RunKind %q, got %#v", *tc.wantRunKind, filter.RunKind)
			}
		})
	}
}

// TestParseRunListFilterChatThreadID asserts the chat_thread_id query
// parameter parses into DigitalEmployeeRunListFilter.ChatThreadID: absent
// means nil, a valid uuid is forwarded, and garbage is a 400 message.
func TestParseRunListFilterChatThreadID(t *testing.T) {
	threadID := uuid.New()

	cases := []struct {
		name       string
		query      string
		wantThread *uuid.UUID
		wantErr    string
	}{
		{name: "absent", query: "", wantThread: nil, wantErr: ""},
		{name: "valid", query: "chat_thread_id=" + threadID.String(), wantThread: &threadID, wantErr: ""},
		{name: "invalid", query: "chat_thread_id=not-a-uuid", wantThread: nil, wantErr: "chat_thread_id must be a valid uuid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/runs?"+tc.query, nil)
			filter, parseErr := parseRunListFilter(r)
			if parseErr != tc.wantErr {
				t.Fatalf("expected parse error %q, got %q", tc.wantErr, parseErr)
			}
			if tc.wantThread == nil {
				if filter.ChatThreadID != nil {
					t.Fatalf("expected nil ChatThreadID, got %s", *filter.ChatThreadID)
				}
				return
			}
			if filter.ChatThreadID == nil || *filter.ChatThreadID != *tc.wantThread {
				t.Fatalf("expected ChatThreadID %s, got %#v", *tc.wantThread, filter.ChatThreadID)
			}
		})
	}
}
