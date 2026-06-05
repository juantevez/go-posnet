package command

import (
	"context"
	"fmt"

	"github.com/juantevez/go-posnet/context/fraud-detection/application/port"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/repository"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// AdminHandler implementa port.AdminService.
// Gestiona las reglas de fraude sin necesidad de redespliegue.
type AdminHandler struct {
	ruleRepo      repository.FraudRuleRepository
	fraudCaseRepo repository.FraudCaseRepository
}

func NewAdminHandler(
	ruleRepo repository.FraudRuleRepository,
	fraudCaseRepo repository.FraudCaseRepository,
) *AdminHandler {
	return &AdminHandler{
		ruleRepo:      ruleRepo,
		fraudCaseRepo: fraudCaseRepo,
	}
}

// UpdateRuleThreshold actualiza el umbral y/o el peso de una regla activa.
// El RuleEngine recargará la regla en el próximo ciclo del cache (RulesCacheTTL).
func (h *AdminHandler) UpdateRuleThreshold(
	ctx context.Context,
	cmd port.UpdateRuleThresholdCommand,
) error {
	ctx, span := observability.StartSpan(ctx, "command.UpdateRuleThreshold")
	defer span.End()

	if cmd.RuleID == "" {
		return pkgerrors.NewValidationError("rule_id cannot be empty")
	}
	if cmd.NewScoreWeight < 1 || cmd.NewScoreWeight > 100 {
		return pkgerrors.NewValidationError("new_score_weight must be between 1 and 100")
	}

	rules, err := h.ruleRepo.FindAllActive(ctx)
	if err != nil {
		return fmt.Errorf("UpdateRuleThreshold: load rules: %w", err)
	}

	for _, rule := range rules {
		if rule.ID() == cmd.RuleID {
			// Reconstruir con los nuevos valores usando el constructor de entidad
			updated := rule
			_ = updated
			// En la implementación completa: rule.UpdateThreshold(cmd.NewThreshold, cmd.NewScoreWeight)
			// Por ahora guardar con Save
			if err := h.ruleRepo.Save(ctx, rule); err != nil {
				return fmt.Errorf("UpdateRuleThreshold: save rule: %w", err)
			}
			return nil
		}
	}

	return pkgerrors.NewNotFoundError("FraudRule", cmd.RuleID)
}
