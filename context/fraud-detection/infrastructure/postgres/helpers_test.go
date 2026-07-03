package postgres_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/aggregate"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/entity"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// newMockPool crea un pool pgxmock y registra su cierre y la verificación de
// expectations al finalizar el test.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if err := pool.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet pgxmock expectations: %v", err)
		}
	})
	return pool
}

// anyArgs devuelve n comodines pgxmock.AnyArg(). pgxmock exige que la
// expectation declare exactamente la misma cantidad de argumentos que la
// llamada real (omitir WithArgs equivale a esperar cero argumentos).
func anyArgs(n int) []any {
	args := make([]any, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

// jsonArg matchea un argumento []byte comparando su contenido JSON decodificado
// contra want, en vez de exigir igualdad byte a byte — evita depender del
// orden exacto de bytes que produce json.Marshal.
type jsonArg struct{ want any }

func (j jsonArg) Match(v any) bool {
	raw, ok := v.([]byte)
	if !ok {
		return false
	}
	var got any
	if err := json.Unmarshal(raw, &got); err != nil {
		return false
	}
	wantRaw, err := json.Marshal(j.want)
	if err != nil {
		return false
	}
	var want any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		return false
	}
	return reflect.DeepEqual(got, want)
}

// ─── builders ────────────────────────────────────────────────────────────────

func mustFraudScore(t *testing.T, score int, rulesHit []string) valueobject.FraudScore {
	t.Helper()
	s, err := valueobject.NewFraudScore(score, rulesHit)
	if err != nil {
		t.Fatalf("NewFraudScore() error = %v", err)
	}
	return s
}

func mustFraudRule(t *testing.T, id string, scoreWeight int) *entity.FraudRule {
	t.Helper()
	r, err := entity.NewFraudRule(id, "Rule "+id, "description", scoreWeight, 0)
	if err != nil {
		t.Fatalf("NewFraudRule(%q) error = %v", id, err)
	}
	return r
}

func mustRuleEval(t *testing.T, ruleID string, activated bool, score int, reason string) valueobject.RuleEvaluation {
	t.Helper()
	e, err := valueobject.NewRuleEvaluation(ruleID, "Rule "+ruleID, activated, score, reason)
	if err != nil {
		t.Fatalf("NewRuleEvaluation(%q) error = %v", ruleID, err)
	}
	return e
}

// newFraudCase construye un FraudCase reconstituido con evaluaciones, listo
// para pasar a FraudCaseRepo.Save.
func newFraudCase(t *testing.T, txID domain.TransactionID) *aggregate.FraudCase {
	t.Helper()
	evaluatedAt := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            "fraud-case-1",
		TransactionID: txID,
		TerminalID:    domain.NewTerminalID(),
		MerchantID:    domain.NewMerchantID(),
		AmountCents:   5000,
		Currency:      "ARS",
		CardNetwork:   "VISA",
		EntryMode:     "CHIP",
		OccurredAt:    time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Score:         mustFraudScore(t, 30, []string{"RULE-001"}),
		Evaluations: []valueobject.RuleEvaluation{
			mustRuleEval(t, "RULE-001", true, 30, "high velocity"),
		},
		EvaluatedAt: &evaluatedAt,
	})
}
