package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/juantevez/go-posnet/pkg/observability"
)

// recoverMiddleware captura panics en los handlers HTTP y retorna
// un 500 con cuerpo JSON en lugar de caer el proceso completo.
// Loguea el stack trace completo para diagnóstico.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()

				log := observability.FromContext(r.Context())
				log.Error("panic recovered in HTTP handler",
					slog.Any("panic", rec),
					slog.String("stack", string(stack)),
					slog.String("path", r.URL.Path),
					slog.String("method", r.Method),
				)

				observability.RecordError(r.Context(), fmt.Errorf("panic: %v", rec))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":   "INTERNAL_ERROR",
					"message": "an unexpected error occurred",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
