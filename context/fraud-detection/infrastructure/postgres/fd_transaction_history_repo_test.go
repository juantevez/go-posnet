package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/fraud-detection/infrastructure/postgres"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func TestTransactionHistoryRepo_CountByTerminalLastHour(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pool := newMockPool(t)
		terminalID := domain.NewTerminalID()
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(terminalID.String()).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(42))

		repo := postgres.NewTransactionHistoryRepo(pool)
		count, err := repo.CountByTerminalLastHour(context.Background(), terminalID)
		if err != nil {
			t.Fatalf("CountByTerminalLastHour() error = %v", err)
		}
		if count != 42 {
			t.Errorf("count = %d, want 42", count)
		}
	})

	t.Run("error", func(t *testing.T) {
		pool := newMockPool(t)
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(anyArgs(1)...).
			WillReturnError(errors.New("connection reset"))

		repo := postgres.NewTransactionHistoryRepo(pool)
		_, err := repo.CountByTerminalLastHour(context.Background(), domain.NewTerminalID())
		if err == nil || !strings.Contains(err.Error(), "TransactionHistoryRepo.CountByTerminalLastHour") {
			t.Fatalf("error = %v, want it to contain %q", err, "TransactionHistoryRepo.CountByTerminalLastHour")
		}
	})
}

func TestTransactionHistoryRepo_AverageAmountByMerchant(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pool := newMockPool(t)
		merchantID := domain.NewMerchantID()
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(merchantID.String()).
			WillReturnRows(pgxmock.NewRows([]string{"avg"}).AddRow(int64(12345)))

		repo := postgres.NewTransactionHistoryRepo(pool)
		avg, err := repo.AverageAmountByMerchant(context.Background(), merchantID)
		if err != nil {
			t.Fatalf("AverageAmountByMerchant() error = %v", err)
		}
		if avg != 12345 {
			t.Errorf("avg = %d, want 12345", avg)
		}
	})

	t.Run("error", func(t *testing.T) {
		pool := newMockPool(t)
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(anyArgs(1)...).
			WillReturnError(errors.New("connection reset"))

		repo := postgres.NewTransactionHistoryRepo(pool)
		_, err := repo.AverageAmountByMerchant(context.Background(), domain.NewMerchantID())
		if err == nil || !strings.Contains(err.Error(), "TransactionHistoryRepo.AverageAmountByMerchant") {
			t.Fatalf("error = %v, want it to contain %q", err, "TransactionHistoryRepo.AverageAmountByMerchant")
		}
	})
}

func TestTransactionHistoryRepo_CountRecentRejectionsByTerminal(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pool := newMockPool(t)
		terminalID := domain.NewTerminalID()
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(terminalID.String(), 10).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(4))

		repo := postgres.NewTransactionHistoryRepo(pool)
		count, err := repo.CountRecentRejectionsByTerminal(context.Background(), terminalID, 10)
		if err != nil {
			t.Fatalf("CountRecentRejectionsByTerminal() error = %v", err)
		}
		if count != 4 {
			t.Errorf("count = %d, want 4", count)
		}
	})

	t.Run("error", func(t *testing.T) {
		pool := newMockPool(t)
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(anyArgs(2)...).
			WillReturnError(errors.New("connection reset"))

		repo := postgres.NewTransactionHistoryRepo(pool)
		_, err := repo.CountRecentRejectionsByTerminal(context.Background(), domain.NewTerminalID(), 10)
		if err == nil || !strings.Contains(err.Error(), "TransactionHistoryRepo.CountRecentRejectionsByTerminal") {
			t.Fatalf("error = %v, want it to contain %q", err, "TransactionHistoryRepo.CountRecentRejectionsByTerminal")
		}
	})
}

func TestTransactionHistoryRepo_CountSameAmountAttempts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pool := newMockPool(t)
		terminalID := domain.NewTerminalID()
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(terminalID.String(), int64(5000), 5).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))

		repo := postgres.NewTransactionHistoryRepo(pool)
		count, err := repo.CountSameAmountAttempts(context.Background(), terminalID, 5000, 5)
		if err != nil {
			t.Fatalf("CountSameAmountAttempts() error = %v", err)
		}
		if count != 2 {
			t.Errorf("count = %d, want 2", count)
		}
	})

	t.Run("error", func(t *testing.T) {
		pool := newMockPool(t)
		pool.ExpectQuery("FROM pn_authorization.transactions").
			WithArgs(anyArgs(3)...).
			WillReturnError(errors.New("connection reset"))

		repo := postgres.NewTransactionHistoryRepo(pool)
		_, err := repo.CountSameAmountAttempts(context.Background(), domain.NewTerminalID(), 5000, 5)
		if err == nil || !strings.Contains(err.Error(), "TransactionHistoryRepo.CountSameAmountAttempts") {
			t.Fatalf("error = %v, want it to contain %q", err, "TransactionHistoryRepo.CountSameAmountAttempts")
		}
	})
}
