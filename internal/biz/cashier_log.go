package biz

import (
	"context"
	"time"

	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/ent/enum"
	"github.com/makesalekz/sales/internal/data"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
)

// CashierLogger is a best-effort logger injected into usecases.
// Errors are logged but do not fail the parent operation.
type CashierLogger interface {
	Log(ctx context.Context, entry CashierLogEntry)
}

type CashierLogEntry struct {
	TenantID    int64
	CashierID   int64
	ShiftID     int64
	Action      enum.CashierLogAction
	EntityID    int64
	EntityType  string
	Amount      decimal.Decimal
	Description string
}

type cashierLogger struct {
	repo data.CashierLogRepo
	log  *log.Helper
}

func NewCashierLogger(repo data.CashierLogRepo, logger log.Logger) CashierLogger {
	return &cashierLogger{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

func (l *cashierLogger) Log(ctx context.Context, entry CashierLogEntry) {
	_, err := l.repo.Create(ctx, data.CashierLogDto{
		TenantID:    entry.TenantID,
		CashierID:   entry.CashierID,
		ShiftID:     entry.ShiftID,
		Action:      entry.Action,
		EntityID:    entry.EntityID,
		EntityType:  entry.EntityType,
		Amount:      entry.Amount,
		Description: entry.Description,
	})
	if err != nil {
		l.log.Errorf("failed to write cashier log: %v", err)
	}
}

// CashierLogUsecase handles GetCashierLog queries.
type CashierLogUsecase struct {
	repo data.CashierLogRepo
}

func NewCashierLogUsecase(repo data.CashierLogRepo) *CashierLogUsecase {
	return &CashierLogUsecase{repo: repo}
}

type CashierLogFilter struct {
	TenantID  int64
	CashierID int64
	ShiftID   int64
	DateFrom  *time.Time
	DateTo    *time.Time
	Limit     int32
	FromID    int64
}

func (uc *CashierLogUsecase) GetCashierLog(ctx context.Context, filter CashierLogFilter) ([]*ent.CashierLog, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	return uc.repo.List(ctx, data.CashierLogFilter{
		TenantID:  filter.TenantID,
		CashierID: filter.CashierID,
		ShiftID:   filter.ShiftID,
		DateFrom:  filter.DateFrom,
		DateTo:    filter.DateTo,
		Limit:     filter.Limit,
		FromID:    filter.FromID,
	})
}
