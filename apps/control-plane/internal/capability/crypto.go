package capability

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const aesGCMCredentialPrefix = "aesgcm:v1:"

type CredentialSealer interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

type AESGCMCredentialSealer struct {
	aead cipher.AEAD
}

func NewAESGCMCredentialSealer(encodedKey string) (*AESGCMCredentialSealer, error) {
	if strings.TrimSpace(encodedKey) == "" {
		return nil, ErrCredentialKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("%w: key must be base64 encoded", ErrInvalidInput)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: key must decode to 32 bytes", ErrInvalidInput)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot create cipher", ErrInvalidInput)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot create gcm", ErrInvalidInput)
	}
	return &AESGCMCredentialSealer{aead: aead}, nil
}

func (s *AESGCMCredentialSealer) Seal(plain string) (string, error) {
	if s == nil || s.aead == nil {
		return "", ErrCredentialKeyMissing
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := s.aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return aesGCMCredentialPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *AESGCMCredentialSealer) Open(sealed string) (string, error) {
	if s == nil || s.aead == nil {
		return "", ErrCredentialKeyMissing
	}
	if !strings.HasPrefix(sealed, aesGCMCredentialPrefix) {
		return "", fmt.Errorf("%w: invalid sealed credential prefix", ErrInvalidInput)
	}
	encodedPayload := strings.TrimPrefix(sealed, aesGCMCredentialPrefix)
	payload, err := base64.StdEncoding.DecodeString(encodedPayload)
	if err != nil {
		return "", fmt.Errorf("%w: invalid sealed credential payload", ErrInvalidInput)
	}
	nonceSize := s.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", fmt.Errorf("%w: invalid sealed credential payload", ErrInvalidInput)
	}
	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plain, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: cannot open sealed credential", ErrInvalidInput)
	}
	return string(plain), nil
}
