package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsHandler(origins []string) http.Handler {
	return CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
}

func preflight(t *testing.T, handler http.Handler, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestCORSAllowsConfiguredOriginsWithCredentials(t *testing.T) {
	handler := corsHandler([]string{"https://console.example.com"})

	rr := preflight(t, handler, "https://console.example.com")

	if rr.Header().Get("Access-Control-Allow-Origin") != "https://console.example.com" {
		t.Fatalf("expected configured console origin, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
	if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("expected credentialed CORS response, got %q", rr.Header().Get("Access-Control-Allow-Credentials"))
	}
}

func TestCORSRejectsUnknownCredentialedOrigins(t *testing.T) {
	handler := corsHandler([]string{"https://console.example.com"})

	rr := preflight(t, handler, "http://malicious.example.com")

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected unknown origin to be rejected, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

// 中间件本身不认识任何"开发默认值":来源全部由调用方注入。这条锁住的是
// 「CORS 白名单不得在中间件里私自读环境变量或硬编码本地端口」这个边界——
// 否则生产二进制会永久放行 localhost,任何能诱导用户本地起服务的页面都能
// 带着会话 cookie 打生产 API。
func TestCORSHasNoBuiltInOrigins(t *testing.T) {
	t.Setenv("CONTROL_PLANE_CORS_ALLOWED_ORIGINS", "http://127.0.0.1:3100")
	handler := corsHandler(nil)

	for _, origin := range []string{
		"http://127.0.0.1:3100",
		"http://localhost:3100",
		"http://127.0.0.1:3000",
	} {
		rr := preflight(t, handler, origin)
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("未注入来源时不得放行任何跨源请求,%q 却拿到了 %q", origin, got)
		}
	}
}

// 非跨源请求(无 Origin 头)不受白名单影响,否则同源部署会被自己挡住。
func TestCORSLeavesSameOriginRequestsAlone(t *testing.T) {
	handler := corsHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("同源请求应照常放行,得到 %d", rr.Code)
	}
}
