package valueobject_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
)

func TestParseFraudDecision(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    valueobject.FraudDecision
		wantErr bool
	}{
		{"approve", "APPROVE", valueobject.DecisionApprove, false},
		{"review", "REVIEW", valueobject.DecisionReview, false},
		{"reject", "REJECT", valueobject.DecisionReject, false},
		{"unknown value", "BOGUS", "", true},
		{"empty string", "", "", true},
		{"lowercase not accepted", "approve", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := valueobject.ParseFraudDecision(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseFraudDecision(%q) error = nil, want error", tc.input)
				}
				if !strings.Contains(err.Error(), "unknown fraud decision") {
					t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown fraud decision")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFraudDecision(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseFraudDecision(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestFraudDecision_String(t *testing.T) {
	if got := valueobject.DecisionReview.String(); got != "REVIEW" {
		t.Errorf("String() = %q, want %q", got, "REVIEW")
	}
}

func TestFraudDecision_ShouldReject(t *testing.T) {
	tests := []struct {
		decision valueobject.FraudDecision
		want     bool
	}{
		{valueobject.DecisionReject, true},
		{valueobject.DecisionApprove, false},
		{valueobject.DecisionReview, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.decision), func(t *testing.T) {
			if got := tc.decision.ShouldReject(); got != tc.want {
				t.Errorf("%s.ShouldReject() = %v, want %v", tc.decision, got, tc.want)
			}
		})
	}
}

func TestNewFraudScore_DecisionThresholds(t *testing.T) {
	tests := []struct {
		score        int
		wantDecision valueobject.FraudDecision
	}{
		{0, valueobject.DecisionApprove},
		{49, valueobject.DecisionApprove},
		{50, valueobject.DecisionReview},
		{69, valueobject.DecisionReview},
		{70, valueobject.DecisionReject},
		{100, valueobject.DecisionReject},
	}

	for _, tc := range tests {
		t.Run(tc.wantDecision.String(), func(t *testing.T) {
			fs, err := valueobject.NewFraudScore(tc.score, nil)
			if err != nil {
				t.Fatalf("NewFraudScore(%d) error = %v", tc.score, err)
			}
			if fs.Decision() != tc.wantDecision {
				t.Errorf("NewFraudScore(%d).Decision() = %v, want %v", tc.score, fs.Decision(), tc.wantDecision)
			}
		})
	}
}

func TestNewFraudScore_OutOfRange(t *testing.T) {
	tests := []int{-1, 101, -100, 1000}

	for _, score := range tests {
		_, err := valueobject.NewFraudScore(score, nil)
		if err == nil {
			t.Fatalf("NewFraudScore(%d) error = nil, want error", score)
		}
		if !strings.Contains(err.Error(), "out of range") {
			t.Errorf("NewFraudScore(%d) error = %q, want it to contain %q", score, err.Error(), "out of range")
		}
	}
}

func TestFraudScore_Getters(t *testing.T) {
	rulesHit := []string{"R1", "R2"}
	fs, err := valueobject.NewFraudScore(75, rulesHit)
	if err != nil {
		t.Fatalf("NewFraudScore() error = %v", err)
	}
	if fs.Score() != 75 {
		t.Errorf("Score() = %d, want 75", fs.Score())
	}
	if fs.Decision() != valueobject.DecisionReject {
		t.Errorf("Decision() = %v, want %v", fs.Decision(), valueobject.DecisionReject)
	}
	if len(fs.RulesHit()) != 2 || fs.RulesHit()[0] != "R1" || fs.RulesHit()[1] != "R2" {
		t.Errorf("RulesHit() = %v, want %v", fs.RulesHit(), rulesHit)
	}
	if !fs.ShouldReject() {
		t.Error("ShouldReject() = false, want true")
	}
	if fs.IsZero() {
		t.Error("IsZero() = true, want false")
	}
}

func TestFraudScore_IsZero(t *testing.T) {
	var zero valueobject.FraudScore
	if !zero.IsZero() {
		t.Error("zero-value FraudScore.IsZero() = false, want true")
	}

	fs, err := valueobject.NewFraudScore(10, nil)
	if err != nil {
		t.Fatalf("NewFraudScore() error = %v", err)
	}
	if fs.IsZero() {
		t.Error("initialized FraudScore.IsZero() = true, want false")
	}
}
