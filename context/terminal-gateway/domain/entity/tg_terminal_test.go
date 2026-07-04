package entity_test

import (
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/entity"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── NewTerminal ───────────────────────────────────────────────────────────────

func TestNewTerminal_Success(t *testing.T) {
	id := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()

	before := time.Now().UTC()
	term, err := entity.NewTerminal(id, merchantID, "TRM-0042", "terminal.example.com")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("NewTerminal() error = %v", err)
	}

	if !term.ID().Equals(id) {
		t.Errorf("ID() = %v, want %v", term.ID(), id)
	}
	if !term.MerchantID().Equals(merchantID) {
		t.Errorf("MerchantID() = %v, want %v", term.MerchantID(), merchantID)
	}
	if term.TerminalCode() != "TRM-0042" {
		t.Errorf("TerminalCode() = %q, want %q", term.TerminalCode(), "TRM-0042")
	}
	if term.CertificateCN() != "terminal.example.com" {
		t.Errorf("CertificateCN() = %q, want %q", term.CertificateCN(), "terminal.example.com")
	}
	if term.Status() != entity.TerminalActive {
		t.Errorf("Status() = %v, want %v", term.Status(), entity.TerminalActive)
	}
	if term.CreatedAt().Before(before) || term.CreatedAt().After(after) {
		t.Errorf("CreatedAt() = %v, want between %v and %v", term.CreatedAt(), before, after)
	}
	if term.UpdatedAt().Before(before) || term.UpdatedAt().After(after) {
		t.Errorf("UpdatedAt() = %v, want between %v and %v", term.UpdatedAt(), before, after)
	}
	if !term.IsActive() {
		t.Error("IsActive() = false, want true right after creation")
	}
}

func TestNewTerminal_ValidationErrors(t *testing.T) {
	tests := []struct {
		name          string
		id            domain.TerminalID
		merchantID    domain.MerchantID
		terminalCode  string
		certificateCN string
		wantSubstr    string
	}{
		{"zero id", domain.TerminalID{}, domain.NewMerchantID(), "TRM-0042", "cn", "id cannot be zero"},
		{"zero merchant_id", domain.NewTerminalID(), domain.MerchantID{}, "TRM-0042", "cn", "merchant_id cannot be zero"},
		{"empty terminal_code", domain.NewTerminalID(), domain.NewMerchantID(), "", "cn", "terminal_code cannot be empty"},
		{"empty certificate_cn", domain.NewTerminalID(), domain.NewMerchantID(), "TRM-0042", "", "certificate_cn cannot be empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := entity.NewTerminal(tc.id, tc.merchantID, tc.terminalCode, tc.certificateCN)
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantSubstr)
			}
		})
	}
}

// ─── ReconstitueTerminal ────────────────────────────────────────────────────────

func TestReconstitueTerminal(t *testing.T) {
	id := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	createdAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	term := entity.ReconstitueTerminal(id, merchantID, "TRM-0042", "terminal.example.com", entity.TerminalBlocked, createdAt, updatedAt)

	if !term.ID().Equals(id) {
		t.Errorf("ID() = %v, want %v", term.ID(), id)
	}
	if !term.MerchantID().Equals(merchantID) {
		t.Errorf("MerchantID() = %v, want %v", term.MerchantID(), merchantID)
	}
	if term.TerminalCode() != "TRM-0042" {
		t.Errorf("TerminalCode() = %q, want %q", term.TerminalCode(), "TRM-0042")
	}
	if term.CertificateCN() != "terminal.example.com" {
		t.Errorf("CertificateCN() = %q, want %q", term.CertificateCN(), "terminal.example.com")
	}
	if term.Status() != entity.TerminalBlocked {
		t.Errorf("Status() = %v, want %v", term.Status(), entity.TerminalBlocked)
	}
	if !term.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt() = %v, want %v", term.CreatedAt(), createdAt)
	}
	if !term.UpdatedAt().Equal(updatedAt) {
		t.Errorf("UpdatedAt() = %v, want %v", term.UpdatedAt(), updatedAt)
	}
	if term.IsActive() {
		t.Error("IsActive() = true, want false for a BLOCKED terminal")
	}
}

// ─── IsActive ─────────────────────────────────────────────────────────────────

func TestIsActive(t *testing.T) {
	tests := []struct {
		status entity.TerminalStatus
		want   bool
	}{
		{entity.TerminalActive, true},
		{entity.TerminalBlocked, false},
		{entity.TerminalMaintenance, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			term := entity.ReconstitueTerminal(
				domain.NewTerminalID(), domain.NewMerchantID(), "TRM-0042", "cn",
				tc.status, time.Now(), time.Now(),
			)
			if got := term.IsActive(); got != tc.want {
				t.Errorf("IsActive() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ─── Block ────────────────────────────────────────────────────────────────────

func TestBlock(t *testing.T) {
	originalUpdatedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	term := entity.ReconstitueTerminal(
		domain.NewTerminalID(), domain.NewMerchantID(), "TRM-0042", "cn",
		entity.TerminalActive, originalUpdatedAt, originalUpdatedAt,
	)

	term.Block()

	if term.Status() != entity.TerminalBlocked {
		t.Errorf("Status() = %v, want %v", term.Status(), entity.TerminalBlocked)
	}
	if term.IsActive() {
		t.Error("IsActive() = true, want false after Block()")
	}
	if !term.UpdatedAt().After(originalUpdatedAt) {
		t.Errorf("UpdatedAt() = %v, want it updated to a time after %v", term.UpdatedAt(), originalUpdatedAt)
	}
}
