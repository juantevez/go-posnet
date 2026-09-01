package domain_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/pkg/domain"
)

const validToken = "3b1f8a2c9d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8"

func TestNewCardToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tok, err := domain.NewCardToken(validToken)
		if err != nil {
			t.Fatalf("NewCardToken() error = %v", err)
		}
		if tok.IsZero() {
			t.Error("IsZero() = true, want false")
		}
		if tok.String() != validToken {
			t.Errorf("String() = %q, want %q", tok.String(), validToken)
		}
	})

	invalid := map[string]string{
		"empty":      "",
		"too short":  validToken[:63],
		"too long":   validToken + "a",
		"uppercase":  strings.ToUpper(validToken),
		"non hex":    strings.Repeat("z", 64),
		"last4 only": "1234",
	}
	for name, in := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewCardToken(in); err == nil {
				t.Errorf("NewCardToken(%q) error = nil, want an error", in)
			}
		})
	}
}

func TestParseOptionalCardToken(t *testing.T) {
	t.Run("empty means absent, not an error", func(t *testing.T) {
		tok, err := domain.ParseOptionalCardToken("")
		if err != nil {
			t.Fatalf("ParseOptionalCardToken(\"\") error = %v", err)
		}
		if !tok.IsZero() {
			t.Error("IsZero() = false, want true")
		}
	})

	t.Run("malformed is an integration error, not an absence", func(t *testing.T) {
		if _, err := domain.ParseOptionalCardToken("not-a-token"); err == nil {
			t.Error("ParseOptionalCardToken() error = nil, want an error")
		}
	})
}

func TestRequiresCardCapture(t *testing.T) {
	capture := []string{domain.ISOCaptureCard, domain.ISOLostCard, domain.ISOStolenCard, domain.CodeCardBlocked}
	for _, code := range capture {
		if !domain.RequiresCardCapture(code) {
			t.Errorf("RequiresCardCapture(%q) = false, want true", code)
		}
	}

	noCapture := []string{"00", "05", "51", "54", "FRAUD_REJECTED", "TIMEOUT", ""}
	for _, code := range noCapture {
		if domain.RequiresCardCapture(code) {
			t.Errorf("RequiresCardCapture(%q) = true, want false", code)
		}
	}
}
