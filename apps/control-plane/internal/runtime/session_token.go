package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const runtimeSessionTokenBytes = 32

func GenerateRuntimeSessionToken() (string, error) {
	buf := make([]byte, runtimeSessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func GenerateRuntimeSecret() (string, error) {
	return GenerateRuntimeSessionToken()
}

func LookupRuntimeSessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashRuntimeSecret(secret string) (string, error) {
	if secret == "" {
		return "", errors.New("runtime secret is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyRuntimeSecret(secret, hash string) bool {
	if secret == "" || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}
