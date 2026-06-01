package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggerIncludesRequestSource(t *testing.T) {
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
	})

	handler := Logger()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/sync/increment", nil)
	req.RemoteAddr = "192.168.124.60:53171"
	req.Header.Set("User-Agent", "Chrome/148.0.7778.179")
	req.Header.Set("Referer", "http://127.0.0.1:3000/users")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logLine := logs.String()
	for _, want := range []string{
		"POST /api/sync/increment 404",
		`remote="192.168.124.60:53171"`,
		`ua="Chrome/148.0.7778.179"`,
		`referer="http://127.0.0.1:3000/users"`,
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("expected log line to contain %s, got %q", want, logLine)
		}
	}
}
