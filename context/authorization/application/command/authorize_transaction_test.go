package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/authorization/application/command"
	"github.com/juantevez/go-posnet/context/authorization/application/port"
	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func TestAuthorizeTransaction_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*port.AuthorizeTransactionCommand)
		wantErr string
	}{
		{"invalid transaction id", func(c *port.AuthorizeTransactionCommand) { c.TransactionID = "not-a-uuid" }, "invalid transaction_id"},
		{"invalid terminal id", func(c *port.AuthorizeTransactionCommand) { c.TerminalID = "not-a-uuid" }, "invalid terminal_id"},
		{"invalid merchant id", func(c *port.AuthorizeTransactionCommand) { c.MerchantID = "not-a-uuid" }, "invalid merchant_id"},
		{"invalid currency", func(c *port.AuthorizeTransactionCommand) { c.Currency = "XXX" }, "invalid currency"},
		{"invalid amount", func(c *port.AuthorizeTransactionCommand) { c.AmountCents = 0 }, "invalid amount"},
		{"invalid stan", func(c *port.AuthorizeTransactionCommand) { c.STAN = 0 }, "invalid stan"},
		{"invalid card network", func(c *port.AuthorizeTransactionCommand) { c.CardNetwork = "BOGUS" }, "invalid card_network"},
		{"invalid pan", func(c *port.AuthorizeTransactionCommand) { c.CardLast4 = "12" }, "invalid pan"},
		{"invalid entry mode", func(c *port.AuthorizeTransactionCommand) { c.EntryMode = "SWIPE" }, "invalid entry_mode"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validAuthorizeCmd(t)
			tc.mutate(&cmd)

			h := command.NewAuthorizationHandler(
				&fakeRepo{}, &fakeAcquirer{}, &fakePublisher{},
				natsutil.NewIdempotencyStore(nil, idempotencySchema),
				nil, // el pool no debe tocarse: la validación falla antes de llegar a la DB
			)

			err := h.AuthorizeTransaction(context.Background(), cmd)
			if err == nil {
				t.Fatalf("AuthorizeTransaction() error = nil, want error containing %q", tc.wantErr)
			}
			var ve *pkgerrors.ValidationError
			if !errors.As(err, &ve) {
				t.Errorf("error type = %T, want *pkgerrors.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestAuthorizeTransaction_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{}
	publisher := &fakePublisher{}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.AuthorizeTransaction(context.Background(), validAuthorizeCmd(t)); err != nil {
		t.Fatalf("AuthorizeTransaction() error = %v", err)
	}

	if len(repo.savedTxs) != 1 {
		t.Fatalf("saved txs = %d, want 1", len(repo.savedTxs))
	}
	if repo.savedTxs[0].State() != valueobject.StateFraudChecking {
		t.Errorf("saved tx state = %v, want %v", repo.savedTxs[0].State(), valueobject.StateFraudChecking)
	}
	if publisher.fraudCheckCalls != 1 {
		t.Errorf("PublishFraudCheckRequested calls = %d, want 1", publisher.fraudCheckCalls)
	}
}

func TestAuthorizeTransaction_DuplicateEventIsSkipped(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 0) // 0 filas afectadas → event_id ya existía

	repo := &fakeRepo{}
	publisher := &fakePublisher{}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.AuthorizeTransaction(context.Background(), validAuthorizeCmd(t)); err != nil {
		t.Fatalf("AuthorizeTransaction() error = %v", err)
	}
	if len(repo.savedTxs) != 0 {
		t.Errorf("saved txs = %d, want 0 (duplicate must not be persisted)", len(repo.savedTxs))
	}
	if publisher.fraudCheckCalls != 0 {
		t.Errorf("PublishFraudCheckRequested calls = %d, want 0", publisher.fraudCheckCalls)
	}
}

func TestAuthorizeTransaction_SaveErrorRollsBack(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO " + idempotencySchema + ".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback() // fn devuelve error → no hay Commit, solo el Rollback diferido

	repo := &fakeRepo{saveErr: errors.New("db unreachable")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.AuthorizeTransaction(context.Background(), validAuthorizeCmd(t))
	if err == nil {
		t.Fatal("AuthorizeTransaction() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "save transaction") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "save transaction")
	}
}

func TestAuthorizeTransaction_PublishFraudCheckErrorIsNonFatal(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{}
	publisher := &fakePublisher{fraudCheckErr: errors.New("nats unavailable")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.AuthorizeTransaction(context.Background(), validAuthorizeCmd(t)); err != nil {
		t.Fatalf("AuthorizeTransaction() error = %v, want nil (publish failure must not fail the saga)", err)
	}
	if len(repo.savedTxs) != 1 {
		t.Errorf("saved txs = %d, want 1", len(repo.savedTxs))
	}
}

func TestAuthorizeTransaction_BeginTxError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).WillReturnError(errors.New("connection refused"))

	h := command.NewAuthorizationHandler(
		&fakeRepo{}, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.AuthorizeTransaction(context.Background(), validAuthorizeCmd(t))
	if err == nil {
		t.Fatal("AuthorizeTransaction() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "begin transaction") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "begin transaction")
	}
}
