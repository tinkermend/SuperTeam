package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
)

var ErrInvalidToken = errors.New("invalid token")

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
