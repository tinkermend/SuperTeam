package auth

import "errors"

var (
	// ErrInvalidCredentials 用户名或密码错误
	ErrInvalidCredentials = errors.New("invalid username or password")

	// ErrUserDisabled 用户账号已被禁用
	ErrUserDisabled = errors.New("user account is disabled")

	// ErrSessionExpired 会话已过期
	ErrSessionExpired = errors.New("session expired")

	// ErrSessionNotFound 会话不存在
	ErrSessionNotFound = errors.New("session not found")

	// ErrUnauthorized 未授权
	ErrUnauthorized = errors.New("unauthorized")
)
