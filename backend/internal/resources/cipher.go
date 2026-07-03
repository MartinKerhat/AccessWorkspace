package resources

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const encryptedSecretPrefix = "enc:v1:"

type SecretCipher struct {
	key []byte
}

func NewSecretCipher(rawKey string) *SecretCipher {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawKey)))
	return &SecretCipher{key: sum[:]}
}

func (c *SecretCipher) EncryptForStorage(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(trimmed), nil)
	return encryptedSecretPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *SecretCipher) DecryptFromStorage(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if !strings.HasPrefix(trimmed, encryptedSecretPrefix) {
		return trimmed, nil
	}
	encoded := strings.TrimPrefix(trimmed, encryptedSecretPrefix)
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("encrypted secret payload is invalid")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
