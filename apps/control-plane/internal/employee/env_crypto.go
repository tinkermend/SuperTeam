package employee

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

type EnvironmentValueCodecConfig struct {
	Keys        string
	ActiveKeyID string
}

type EnvironmentEncryptedValue struct {
	KeyID          string
	EncryptedValue string
	Fingerprint    string
}

type EnvironmentValueCodec struct {
	activeKeyID string
	keys        map[string][]byte
}

func NewEnvironmentValueCodec(cfg EnvironmentValueCodecConfig) (*EnvironmentValueCodec, error) {
	keys := map[string][]byte{}
	for _, item := range strings.Split(cfg.Keys, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("%w: invalid environment encryption key entry", ErrInvalidInput)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("%w: decode environment encryption key: %v", ErrInvalidInput, err)
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("%w: environment encryption key must be 32 bytes", ErrInvalidInput)
		}
		keys[strings.TrimSpace(parts[0])] = raw
	}
	active := strings.TrimSpace(cfg.ActiveKeyID)
	if active == "" {
		return nil, fmt.Errorf("%w: active environment encryption key id is required", ErrInvalidInput)
	}
	if _, ok := keys[active]; !ok {
		return nil, fmt.Errorf("%w: active key is not configured for environment encryption", ErrInvalidInput)
	}
	return &EnvironmentValueCodec{activeKeyID: active, keys: keys}, nil
}

func (c *EnvironmentValueCodec) Encrypt(name, value string) (EnvironmentEncryptedValue, error) {
	key := c.keys[c.activeKeyID]
	block, err := aes.NewCipher(key)
	if err != nil {
		return EnvironmentEncryptedValue{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EnvironmentEncryptedValue{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EnvironmentEncryptedValue{}, err
	}
	aad := []byte(strings.TrimSpace(name))
	ciphertext := gcm.Seal(nil, nonce, []byte(value), aad)
	sealed := append(nonce, ciphertext...)
	return EnvironmentEncryptedValue{
		KeyID:          c.activeKeyID,
		EncryptedValue: base64.StdEncoding.EncodeToString(sealed),
		Fingerprint:    fingerprintValue(key, name, value),
	}, nil
}

func (c *EnvironmentValueCodec) Decrypt(name string, value EnvironmentEncryptedValue) (string, error) {
	key, ok := c.keys[value.KeyID]
	if !ok {
		return "", fmt.Errorf("%w: environment encryption key is not configured", ErrInvalidInput)
	}
	sealed, err := base64.StdEncoding.DecodeString(value.EncryptedValue)
	if err != nil {
		return "", fmt.Errorf("%w: decode encrypted environment value: %v", ErrInvalidInput, err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", fmt.Errorf("%w: encrypted environment value is truncated", ErrInvalidInput)
	}
	nonce := sealed[:gcm.NonceSize()]
	ciphertext := sealed[gcm.NonceSize():]
	aad := []byte(strings.TrimSpace(name))
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt environment value", ErrInvalidInput)
	}
	return string(plain), nil
}

func fingerprintValue(key []byte, name, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(strings.TrimSpace(name)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum)[:12]
}
