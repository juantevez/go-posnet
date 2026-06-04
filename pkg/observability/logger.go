// Package observability centraliza la inicialización de OpenTelemetry,
// el logger estructurado y la propagación de contexto para todos los BCs.
package observability

import (
	"context"
	"log/slog"
	"os"
)

type contextKey string

const loggerKey contextKey = "logger"

// InitLogger configura el logger global con el formato adecuado al entorno.
// En producción: JSON. En desarrollo: texto con colores.
func InitLogger(env string, level slog.Level) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level, AddSource: true}

	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// WithContext retorna un contexto enriquecido con el logger y atributos dados.
// Uso: ctx = observability.WithContext(ctx, slog.String("terminal_id", tid))
func WithContext(ctx context.Context, attrs ...slog.Attr) context.Context {
	logger := FromContext(ctx)
	logger = logger.With(attrsToArgs(attrs)...)
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext extrae el logger del contexto.
// Si no hay logger en el contexto, devuelve el logger por defecto.
// Inyecta automáticamente trace_id y span_id si hay un span activo.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return enrichWithTrace(ctx, logger)
	}
	return enrichWithTrace(ctx, slog.Default())
}

// attrsToArgs convierte []slog.Attr a []any para logger.With().
func attrsToArgs(attrs []slog.Attr) []any {
	args := make([]any, 0, len(attrs)*2)
	for _, a := range attrs {
		args = append(args, a.Key, a.Value.Any())
	}
	return args
}
