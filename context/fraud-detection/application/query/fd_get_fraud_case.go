// Package query contiene los query handlers del BC Fraud Detection.
package query

import (
	"context"
	"fmt"

	"github.com/juantevez/go-posnet/context/fraud-detection/application/port"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/repository"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// FraudQueryHandler implementa las consultas de solo lectura del BC.
type FraudQueryHandler struct {
	fraudCaseRepo repository.FraudCaseRepository
	ruleRepo      repository.FraudRuleRepository
}

func NewFraudQueryHandler(
	fraudCaseRepo repository.FraudCaseRepository,
	ruleRepo repository.FraudRuleRepository,
) *FraudQueryHandler {
	return &FraudQueryHandler{
		fraudCaseRepo: fraudCaseRepo,
		ruleRepo:      ruleRepo,
	}
}

// GetFraudCase retorna el análisis de fraude de una transacción por su ID.
func (h *FraudQueryHandler) GetFraudCase(
	ctx context.Context,
	transactionID string,
) (*port.FraudCaseResult, error) {
	ctx, span := observability.StartSpan(ctx, "query.GetFraudCase")
	defer span.End()

	txID, err := domain.ParseTransactionID(transactionID)
	if err != nil {
		return nil, pkgerrors.NewValidationError("invalid transaction_id: " + err.Error())
	}

	fc, err := h.fraudCaseRepo.FindByTransactionID(ctx, txID)
	if err != nil {
		return nil, fmt.Errorf("GetFraudCase: %w", err)
	}
	if fc == nil {
		return nil, pkgerrors.NewNotFoundError("FraudCase", transactionID)
	}

	// Mapear evaluaciones individuales
	evalResults := make([]port.RuleEvaluationResult, 0, len(fc.Evaluations()))
	for _, eval := range fc.Evaluations() {
		evalResults = append(evalResults, port.RuleEvaluationResult{
			RuleID:            eval.RuleID(),
			RuleName:          eval.RuleName(),
			Activated:         eval.Activated(),
			ScoreContribution: eval.ScoreContribution(),
			Reason:            eval.Reason(),
		})
	}

	result := &port.FraudCaseResult{
		FraudCaseID:   fc.ID(),
		TransactionID: fc.TransactionID().String(),
		Score:         fc.Score().Score(),
		Decision:      fc.Score().Decision().String(),
		RulesHit:      fc.Score().RulesHit(),
		Evaluations:   evalResults,
	}

	if fc.EvaluatedAt() != nil {
		result.EvaluatedAt = fc.EvaluatedAt().Format("2006-01-02T15:04:05Z")
	}

	return result, nil
}

// ListActiveRules retorna todas las reglas de fraude activas.
func (h *FraudQueryHandler) ListActiveRules(ctx context.Context) ([]*port.RuleResult, error) {
	ctx, span := observability.StartSpan(ctx, "query.ListActiveRules")
	defer span.End()

	rules, err := h.ruleRepo.FindAllActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListActiveRules: %w", err)
	}

	results := make([]*port.RuleResult, 0, len(rules))
	for _, r := range rules {
		results = append(results, &port.RuleResult{
			ID:             r.ID(),
			Name:           r.Name(),
			Description:    r.Description(),
			ScoreWeight:    r.ScoreWeight(),
			ThresholdValue: r.ThresholdValue(),
			IsActive:       r.IsActive(),
		})
	}

	return results, nil
}
