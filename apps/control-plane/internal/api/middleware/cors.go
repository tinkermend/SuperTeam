package middleware

import (
	"github.com/go-chi/cors"
	"net/http"
	"os"
	"strings"
)

func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Node-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

func allowedOrigins() []string {
	// 本地规范宿主是 127.0.0.1（Web 入口会把 localhost 跳过去）；仍保留
	// localhost 以免跳转落地前的首请求 CORS 失败。
	origins := []string{"http://127.0.0.1:3000", "http://localhost:3000"}
	for _, origin := range strings.Split(os.Getenv("CONTROL_PLANE_CORS_ALLOWED_ORIGINS"), ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
