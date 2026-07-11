package pgutil

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"

	"github.com/juantevez/go-posnet/pkg/observability"
)

// PoolStatProvider expone Stat() — implementado por *pgxpool.Pool.
// Se define como interface para poder testear con un stub.
type PoolStatProvider interface {
	Stat() *pgxpool.Stat
}

// RegisterPoolMetrics registra métricas observables del pool pgx en el
// MeterProvider global. Debe llamarse una vez por BC, tras
// observability.InitMeter(). El label "bc" lo agrega Prometheus como target
// label (deployments/docker/prometheus.yml), por eso no se agrega aquí.
//
// Nombre final en Prometheus (el exporter agrega _total a counters y _seconds
// a unidades "s"):
//
//	pgx_pool_acquired_conns / _idle_conns / _total_conns / _max_conns  (gauges)
//	pgx_pool_acquire_count_total / _new_conns_total /
//	  _canceled_acquire_total / _empty_acquire_total                   (counters)
//	pgx_pool_acquire_duration_seconds_total                           (counter, wait acumulado)
func RegisterPoolMetrics(pool PoolStatProvider) error {
	m := observability.Meter("posnet.pgxpool")

	acquired, err := m.Int64ObservableGauge("pgx_pool_acquired_conns",
		metric.WithDescription("Conexiones actualmente prestadas del pool."))
	if err != nil {
		return err
	}
	idle, err := m.Int64ObservableGauge("pgx_pool_idle_conns",
		metric.WithDescription("Conexiones idle disponibles en el pool."))
	if err != nil {
		return err
	}
	total, err := m.Int64ObservableGauge("pgx_pool_total_conns",
		metric.WithDescription("Total de conexiones abiertas (acquired + idle + constructing)."))
	if err != nil {
		return err
	}
	maxConns, err := m.Int64ObservableGauge("pgx_pool_max_conns",
		metric.WithDescription("Máximo de conexiones configurado (MaxConns)."))
	if err != nil {
		return err
	}
	acquireCount, err := m.Int64ObservableCounter("pgx_pool_acquire_count",
		metric.WithDescription("Adquisiciones exitosas acumuladas."))
	if err != nil {
		return err
	}
	newConns, err := m.Int64ObservableCounter("pgx_pool_new_conns",
		metric.WithDescription("Conexiones nuevas creadas acumuladas."))
	if err != nil {
		return err
	}
	canceled, err := m.Int64ObservableCounter("pgx_pool_canceled_acquire",
		metric.WithDescription("Adquisiciones canceladas por timeout/contexto."))
	if err != nil {
		return err
	}
	empty, err := m.Int64ObservableCounter("pgx_pool_empty_acquire",
		metric.WithDescription("Adquisiciones que tuvieron que esperar por pool vacío."))
	if err != nil {
		return err
	}
	acquireDur, err := m.Float64ObservableCounter("pgx_pool_acquire_duration",
		metric.WithDescription("Tiempo total acumulado esperando conexiones del pool."),
		metric.WithUnit("s"))
	if err != nil {
		return err
	}

	_, err = m.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		s := pool.Stat()
		o.ObserveInt64(acquired, int64(s.AcquiredConns()))
		o.ObserveInt64(idle, int64(s.IdleConns()))
		o.ObserveInt64(total, int64(s.TotalConns()))
		o.ObserveInt64(maxConns, int64(s.MaxConns()))
		o.ObserveInt64(acquireCount, s.AcquireCount())
		o.ObserveInt64(newConns, s.NewConnsCount())
		o.ObserveInt64(canceled, s.CanceledAcquireCount())
		o.ObserveInt64(empty, s.EmptyAcquireCount())
		o.ObserveFloat64(acquireDur, s.AcquireDuration().Seconds())
		return nil
	}, acquired, idle, total, maxConns, acquireCount, newConns, canceled, empty, acquireDur)
	return err
}
