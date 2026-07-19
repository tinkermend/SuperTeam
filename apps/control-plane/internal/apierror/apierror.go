// Package apierror 提供跨 handler 的结构化 API 错误：机器可读 code + 权威中文
// message。目的是让前端按稳定 code 判断、直接展示后端 message，取代脆弱的
// 「按英文错误文本关键词匹配」。message 是 zh-first 的唯一事实源（后端定义），
// code 供前端需要分支逻辑时使用。
//
// 迁移策略：新错误直接用本包定义 coded error；存量 handler 的 http.Error 纯文本
// 错误按需增量迁移，不要求一次性全改。writeHandlerError 之类的集中出口先接
// apierror.Write，命中结构化错误即输出 JSON，否则退回既有兜底。
package apierror

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Error 是带 code 与用户可读中文 message 的 API 错误。cause 仅用于服务端日志，
// 不外泄给客户端。
type Error struct {
	Code    string
	Status  int
	Message string
	cause   error
}

// New 定义一个 coded error 原型（通常声明为包级变量复用）。
func New(code string, status int, message string) *Error {
	return &Error{Code: code, Status: status, Message: message}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.cause }

// Is 按 Code 匹配，使 WithCause 派生的副本仍能 errors.Is 匹配其原型。
func (e *Error) Is(target error) bool {
	var t *Error
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// WithCause 附加内部成因（进服务端日志），返回副本，不改变对外 code/status/message。
func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// responseBody 是错误响应的线格式：{code, message}。前端 readErrorDetail 读 message
// 作展示文本、读 code 作分支依据。
type responseBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Write 若 err 是/包 *Error，写 {code, message} JSON（application/json + 对应 status）
// 并返回 true；否则不写、返回 false，交调用方兜底（通常 500）。
func Write(w http.ResponseWriter, err error) bool {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		return false
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(responseBody{Code: apiErr.Code, Message: apiErr.Message})
	return true
}
