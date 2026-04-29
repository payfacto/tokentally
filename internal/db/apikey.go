package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

const apiKeyPrefix = "enc:"

// deriveEncKey produces a 32-byte AES-256 key from machine-specific values.
// The key is deterministic per hostname+homedir, so the DB stays portable to
// the same user on the same machine while protecting the key from raw DB reads.
func deriveEncKey() []byte {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	h := sha256.New()
	h.Write([]byte("tokentally-exchange-api-key-v1"))
	h.Write([]byte(hostname))
	h.Write([]byte(home))
	return h.Sum(nil)
}

func encryptAPIKey(plaintext string) (string, error) {
	block, err := aes.NewCipher(deriveEncKey())
	if err != nil {
		return "", fmt.Errorf("encryptAPIKey: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encryptAPIKey gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encryptAPIKey nonce: %w", err)
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return apiKeyPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

func decryptAPIKey(stored string) (string, error) {
	if !strings.HasPrefix(stored, apiKeyPrefix) {
		// Legacy plaintext value — return as-is for backward compatibility.
		return stored, nil
	}
	data, err := base64.StdEncoding.DecodeString(stored[len(apiKeyPrefix):])
	if err != nil {
		return "", fmt.Errorf("decryptAPIKey base64: %w", err)
	}
	block, err := aes.NewCipher(deriveEncKey())
	if err != nil {
		return "", fmt.Errorf("decryptAPIKey: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decryptAPIKey gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("decryptAPIKey: ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decryptAPIKey: %w", err)
	}
	return string(pt), nil
}
