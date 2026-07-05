// Package entity contiene las entidades del BC Fraud Detection.
package entity

import (
	"fmt"
	"time"
)

// FraudRule representa una regla de fraude configurable.
// Se persiste en Postgres y se carga al arrancar el motor.
// Los umbrales son configurables sin necesidad de redespliegue.
type FraudRule struct {
	id             string
	name           string
	description    string
	scoreWeight    int     // Puntos que suma al score si la regla activa
	thresholdValue float64 // Valor configurable del umbral de la regla
	isActive       bool
	updatedAt      time.Time
}

// NewFraudRule crea una regla validando sus invariantes.
func NewFraudRule(id, name, description string, scoreWeight int, thresholdValue float64) (*FraudRule, error) {
	if id == "" {
		return nil, fmt.Errorf("fraud_rule: id cannot be empty")
	}
	if name == "" {
		return nil, fmt.Errorf("fraud_rule: name cannot be empty")
	}
	if scoreWeight <= 0 || scoreWeight > 100 {
		return nil, fmt.Errorf("fraud_rule: score_weight %d out of range [1, 100]", scoreWeight)
	}
	return &FraudRule{
		id:             id,
		name:           name,
		description:    description,
		scoreWeight:    scoreWeight,
		thresholdValue: thresholdValue,
		isActive:       true,
		updatedAt:      time.Now().UTC(),
	}, nil
}

// ReconstituteFraudRule reconstruye una regla desde la base de datos.
func ReconstituteFraudRule(id, name, description string, scoreWeight int, thresholdValue float64, isActive bool, updatedAt time.Time) *FraudRule {
	return &FraudRule{
		id:             id,
		name:           name,
		description:    description,
		scoreWeight:    scoreWeight,
		thresholdValue: thresholdValue,
		isActive:       isActive,
		updatedAt:      updatedAt,
	}
}

func (r *FraudRule) ID() string              { return r.id }
func (r *FraudRule) Name() string            { return r.name }
func (r *FraudRule) Description() string     { return r.description }
func (r *FraudRule) ScoreWeight() int        { return r.scoreWeight }
func (r *FraudRule) ThresholdValue() float64 { return r.thresholdValue }
func (r *FraudRule) IsActive() bool          { return r.isActive }
func (r *FraudRule) UpdatedAt() time.Time    { return r.updatedAt }

// Deactivate desactiva la regla sin eliminarla.
func (r *FraudRule) Deactivate() {
	r.isActive = false
	r.updatedAt = time.Now().UTC()
}

// UpdateThreshold actualiza el umbral y el peso de la regla, validando invariantes.
func (r *FraudRule) UpdateThreshold(newThreshold float64, newScoreWeight int) error {
	if newScoreWeight <= 0 || newScoreWeight > 100 {
		return fmt.Errorf("fraud_rule: score_weight %d out of range [1, 100]", newScoreWeight)
	}
	r.thresholdValue = newThreshold
	r.scoreWeight = newScoreWeight
	r.updatedAt = time.Now().UTC()
	return nil
}
