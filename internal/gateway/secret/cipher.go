package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/viper"
)

const (
	cipherVersion = "v1"
	keyContext    = "done-hub/gateway-secret/v1"
)

var (
	ErrMissingKey        = errors.New("gateway secret key is not configured")
	ErrInvalidCiphertext = errors.New("gateway secret ciphertext is invalid")
)

type Cipher struct {
	aead cipher.AEAD
}

func New(passphrase string) (*Cipher, error) {
	if passphrase == "" {
		return nil, ErrMissingKey
	}
	sum := sha256.Sum256([]byte(keyContext + "\x00" + passphrase))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("create gateway secret cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gateway secret AEAD: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func NewFromConfig() (*Cipher, error) {
	passphrase := viper.GetString("gateway_secret_key")
	if passphrase == "" {
		passphrase = viper.GetString("user_token_secret")
	}
	return New(passphrase)
}

func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate gateway secret nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), []byte(cipherVersion))
	payload := append(nonce, sealed...)
	return cipherVersion + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	prefix := cipherVersion + ":"
	if len(ciphertext) <= len(prefix) || ciphertext[:len(prefix)] != prefix {
		return "", ErrInvalidCiphertext
	}
	payload, err := base64.RawURLEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}
	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", ErrInvalidCiphertext
	}
	plaintext, err := c.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], []byte(cipherVersion))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}
	return string(plaintext), nil
}
