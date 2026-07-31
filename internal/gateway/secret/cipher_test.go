package secret

import (
	"errors"
	"strings"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	t.Parallel()

	cipher, err := New("test-secret-key")
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	plaintext := "credential-value"
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ciphertext == plaintext || strings.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	decrypted, err := cipher.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestCipherRejectsWrongKeyAndInvalidPayload(t *testing.T) {
	t.Parallel()

	first, _ := New("first-key")
	second, _ := New("second-key")
	ciphertext, err := first.Encrypt("credential-value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := second.Decrypt(ciphertext); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("expected invalid ciphertext, got %v", err)
	}
	if _, err := first.Decrypt("plaintext"); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("expected invalid ciphertext, got %v", err)
	}
}

func TestCipherRequiresKey(t *testing.T) {
	t.Parallel()

	if _, err := New(""); !errors.Is(err, ErrMissingKey) {
		t.Fatalf("expected missing key, got %v", err)
	}
}
