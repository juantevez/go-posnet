package websocket

import (
	"context"
	"log/slog"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// MockNotifier es un no-op del TerminalNotifier para desarrollo y MVP.
// Loguea las notificaciones sin enviarlas por WebSocket.
type MockNotifier struct{}

func NewMockNotifier() *MockNotifier {
	return &MockNotifier{}
}

func (m *MockNotifier) NotifySessionCreated(ctx context.Context, session *aggregate.PaymentSession) error {
	slog.Info("mock notifier: session created",
		slog.String("session_id", session.ID().String()),
		slog.Int64("amount_cents", session.Amount().Cents()),
	)
	return nil
}

func (m *MockNotifier) NotifyResult(ctx context.Context, session *aggregate.PaymentSession) error {
	slog.Info("mock notifier: auth result",
		slog.String("session_id", session.ID().String()),
		slog.String("state", session.State().String()),
		slog.Bool("capture_card", session.RequiresCardCapture()),
	)
	return nil
}

func (m *MockNotifier) NotifySessionExpired(ctx context.Context, terminalID domain.TerminalID, transactionID domain.TransactionID) error {
	slog.Info("mock notifier: session expired",
		slog.String("terminal_id", terminalID.String()),
		slog.String("transaction_id", transactionID.String()),
	)
	return nil
}
