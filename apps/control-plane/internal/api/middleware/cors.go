package middleware

import (
	"net/http"
	"strings"

	"github.com/go-chi/cors"
)

// CORS 按传入的允许来源构造中间件。来源由调用方从配置解析(见
// config.Config.ResolvedAllowedOrigins),这里刻意**不读环境变量、不内置任何
// 本地端口**:CORS 白名单是一条承重的安全边界,应当与其余配置走同一套加载与
// 校验路径;在中间件里私自 os.Getenv 会让运维读 config.yaml 看不出放行了谁,
// 而内置 localhost 默认值等于让生产二进制永久放行开发机来源。
//
// allowedOrigins 为空时**拒绝一切跨源来源**。这里必须用 AllowOriginFunc 显式
// 判定,不能只把空切片交给 cors.Options.AllowedOrigins——go-chi/cors 把"没有配
// 置来源"解释为通配 `*`,即空配置反而是最宽松的策略。安全默认值必须是拒绝。
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			allowed[strings.ToLower(trimmed)] = struct{}{}
		}
	}

	return cors.Handler(cors.Options{
		AllowOriginFunc: func(_ *http.Request, origin string) bool {
			_, ok := allowed[strings.ToLower(strings.TrimSpace(origin))]
			return ok
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Node-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
