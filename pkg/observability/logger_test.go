package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// captureStdout redirige os.Stdout durante fn() y devuelve lo escrito.
// InitLogger no acepta un writer inyectable — es la única forma de
// verificar su salida sin tocar el filesystem real.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	fn()

	_ = w.Close()
	os.Stdout = orig
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return string(out)
}

func restoreDefaultLogger(t *testing.T) {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
}

// ─── InitLogger ───────────────────────────────────────────────────────────────

func TestInitLogger_ProductionUsesJSONHandler(t *testing.T) {
	restoreDefaultLogger(t)

	var logger *slog.Logger
	out := captureStdout(t, func() {
		logger = InitLogger("production", slog.LevelInfo)
		logger.Info("hello", slog.String("key", "value"))
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if parsed["msg"] != "hello" {
		t.Errorf("msg = %v, want %q", parsed["msg"], "hello")
	}
	if parsed["key"] != "value" {
		t.Errorf("key = %v, want %q", parsed["key"], "value")
	}
	if _, ok := parsed["source"]; !ok {
		t.Error("source field missing — AddSource should be enabled")
	}
}

func TestInitLogger_NonProductionUsesTextHandler(t *testing.T) {
	restoreDefaultLogger(t)

	var logger *slog.Logger
	out := captureStdout(t, func() {
		logger = InitLogger("development", slog.LevelInfo)
		logger.Info("hello", slog.String("key", "value"))
	})

	if json.Valid([]byte(strings.TrimSpace(out))) {
		t.Errorf("output looks like JSON, want plain text: %s", out)
	}
	if !strings.Contains(out, "msg=hello") {
		t.Errorf("output = %q, want it to contain %q", out, "msg=hello")
	}
	if !strings.Contains(out, "key=value") {
		t.Errorf("output = %q, want it to contain %q", out, "key=value")
	}
}

func TestInitLogger_SetsGlobalDefault(t *testing.T) {
	restoreDefaultLogger(t)

	out := captureStdout(t, func() {
		InitLogger("development", slog.LevelInfo)
		slog.Info("via global default") // no usa el *slog.Logger devuelto — prueba slog.SetDefault()
	})

	if !strings.Contains(out, "via global default") {
		t.Errorf("output = %q, want it to contain the message logged through slog.Info (global default)", out)
	}
}

func TestInitLogger_RespectsLevelFiltering(t *testing.T) {
	restoreDefaultLogger(t)

	var logger *slog.Logger
	out := captureStdout(t, func() {
		logger = InitLogger("development", slog.LevelWarn)
		logger.Info("should be filtered out")
		logger.Warn("should appear")
	})

	if strings.Contains(out, "should be filtered out") {
		t.Errorf("output = %q, want Info-level message to be filtered out at LevelWarn", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("output = %q, want it to contain the Warn-level message", out)
	}
}

// ─── WithContext / FromContext ────────────────────────────────────────────────

func TestWithContext_AddsAttributesToLogger(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, nil))
	ctx := context.WithValue(context.Background(), loggerKey, baseLogger)

	ctx = WithContext(ctx, slog.String("terminal_id", "T-1"))
	FromContext(ctx).Info("test")

	out := buf.String()
	if !strings.Contains(out, "terminal_id=T-1") {
		t.Errorf("output = %q, want it to contain %q", out, "terminal_id=T-1")
	}
}

func TestWithContext_AccumulatesAttributesAcrossNestedCalls(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, nil))
	ctx := context.WithValue(context.Background(), loggerKey, baseLogger)

	ctx = WithContext(ctx, slog.String("terminal_id", "T-1"))
	ctx = WithContext(ctx, slog.String("merchant_id", "M-1"))
	FromContext(ctx).Info("test")

	out := buf.String()
	if !strings.Contains(out, "terminal_id=T-1") {
		t.Errorf("output = %q, want it to contain %q", out, "terminal_id=T-1")
	}
	if !strings.Contains(out, "merchant_id=M-1") {
		t.Errorf("output = %q, want it to contain %q", out, "merchant_id=M-1")
	}
}

func TestFromContext_NoLoggerInContext_FallsBackToDefault(t *testing.T) {
	restoreDefaultLogger(t)
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	slog.SetDefault(custom)

	got := FromContext(context.Background())
	if got != custom {
		t.Error("FromContext() should fall back to slog.Default() when the context has no logger")
	}
}

func TestFromContext_LoggerInContext_ReturnsIt(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.WithValue(context.Background(), loggerKey, custom)

	got := FromContext(ctx)
	if got != custom {
		t.Error("FromContext() should return the logger stored in the context")
	}
}

func TestFromContext_EnrichesWithTraceWhenSpanActive(t *testing.T) {
	setupTestTracing(t)
	ctx, span := StartSpan(context.Background(), "operation")
	defer span.End()

	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, nil))
	ctx = context.WithValue(ctx, loggerKey, baseLogger)

	FromContext(ctx).Info("test")

	traceID := span.SpanContext().TraceID().String()
	if !strings.Contains(buf.String(), "trace_id="+traceID) {
		t.Errorf("output = %q, want it to contain trace_id=%s", buf.String(), traceID)
	}
}

// ─── attrsToArgs ──────────────────────────────────────────────────────────────

func TestAttrsToArgs_ConvertsToKeyValuePairs(t *testing.T) {
	args := attrsToArgs([]slog.Attr{
		slog.String("a", "1"),
		slog.Int("b", 2),
	})

	want := []any{"a", "1", "b", int64(2)}
	if len(args) != len(want) {
		t.Fatalf("len(args) = %d, want %d (args = %v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %v (%T), want %v (%T)", i, args[i], args[i], want[i], want[i])
		}
	}
}

func TestAttrsToArgs_EmptyInput(t *testing.T) {
	args := attrsToArgs(nil)
	if len(args) != 0 {
		t.Errorf("len(args) = %d, want 0", len(args))
	}
}
