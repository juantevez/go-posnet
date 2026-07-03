package http

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/juantevez/go-posnet/context/authorization/application/port"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── fakeQueryService ────────────────────────────────────────────────────────

type fakeQueryService struct {
	result *port.TransactionStatusResult
	err    error
}

var _ port.QueryService = (*fakeQueryService)(nil)

func (f *fakeQueryService) GetTransactionStatus(_ context.Context, _ domain.TransactionID) (*port.TransactionStatusResult, error) {
	return f.result, f.err
}

// panickyQueryService simula un bug en la capa de aplicación para probar que
// recoverMiddleware efectivamente protege al proceso HTTP.
type panickyQueryService struct{}

var _ port.QueryService = (*panickyQueryService)(nil)

func (p *panickyQueryService) GetTransactionStatus(_ context.Context, _ domain.TransactionID) (*port.TransactionStatusResult, error) {
	panic("boom")
}

// ─── fakePool ────────────────────────────────────────────────────────────────

// fakePool implementa pgutil.PgxPool. Solo Ping() se ejercita en este
// paquete — ningún handler HTTP del BC Authorization abre transacciones.
type fakePool struct {
	pingErr error
}

func (f *fakePool) Ping(context.Context) error { return f.pingErr }

func (f *fakePool) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return nil, errors.New("fakePool: BeginTx not implemented")
}

func newTestHandler(qs *fakeQueryService, pool *fakePool) *Handler {
	return NewHandler(qs, pool)
}
