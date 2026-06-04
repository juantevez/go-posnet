package pgutil

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthCheck verifica que la base de datos esté disponible y respondiendo.
// Usado en el endpoint GET /readyz de cada BC para el readiness probe
// de Kubernetes / Docker Compose.
//
// Retorna nil si la BD responde. Retorna error descriptivo si no está disponible.
func HealthCheck(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres health check failed: %w", err)
	}
	return nil
}

// HealthCheckWithTimeout ejecuta el health check con un timeout acotado.
// Evita que un readiness probe cuelgue indefinidamente si la BD no responde.
// timeout recomendado: 2–3 segundos.
func HealthCheckWithTimeout(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return HealthCheck(ctx, pool)
}

// PoolStats retorna estadísticas del pool para métricas de observabilidad.
// Útil para exponer en el endpoint /metrics o en logs periódicos.
type PoolStats struct {
	TotalConns    int32 // Conexiones totales en el pool
	AcquiredConns int32 // Conexiones actualmente en uso
	IdleConns     int32 // Conexiones disponibles
	MaxConns      int32 // Máximo configurado
}

// Stats retorna las estadísticas actuales del pool.
func Stats(pool *pgxpool.Pool) PoolStats {
	s := pool.Stat()
	return PoolStats{
		TotalConns:    s.TotalConns(),
		AcquiredConns: s.AcquiredConns(),
		IdleConns:     s.IdleConns(),
		MaxConns:      s.MaxConns(),
	}
}
