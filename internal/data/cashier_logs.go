package data

import (
	"context"
	"time"

	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/ent/cashierlog"
	"github.com/makesalekz/sales/ent/enum"

	"github.com/shopspring/decimal"
)

type CashierLogDto struct {
	TenantID    int64
	CashierID   int64
	ShiftID     int64
	Action      enum.CashierLogAction
	EntityID    int64
	EntityType  string
	Amount      decimal.Decimal
	Description string
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

type CashierLogRepo interface {
	Create(ctx context.Context, dto CashierLogDto) (*ent.CashierLog, error)
	List(ctx context.Context, filter CashierLogFilter) ([]*ent.CashierLog, error)
}

type cashierLogRepo struct {
	db *ent.Client
}

func NewCashierLogRepo(d *Data) CashierLogRepo {
	return &cashierLogRepo{db: d.db}
}

func (r *cashierLogRepo) Create(ctx context.Context, dto CashierLogDto) (*ent.CashierLog, error) {
	return r.db.CashierLog.Create().
		SetTenantID(dto.TenantID).
		SetCashierID(dto.CashierID).
		SetShiftID(dto.ShiftID).
		SetAction(dto.Action).
		SetEntityID(dto.EntityID).
		SetEntityType(dto.EntityType).
		SetAmount(dto.Amount).
		SetDescription(dto.Description).
		Save(ctx)
}

func (r *cashierLogRepo) List(ctx context.Context, filter CashierLogFilter) ([]*ent.CashierLog, error) {
	q := r.db.CashierLog.Query().
		Where(cashierlog.TenantID(filter.TenantID)).
		Order(ent.Desc(cashierlog.FieldID))

	if filter.CashierID > 0 {
		q = q.Where(cashierlog.CashierID(filter.CashierID))
	}
	if filter.ShiftID > 0 {
		q = q.Where(cashierlog.ShiftID(filter.ShiftID))
	}
	if filter.DateFrom != nil {
		q = q.Where(cashierlog.CreatedAtGTE(*filter.DateFrom))
	}
	if filter.DateTo != nil {
		q = q.Where(cashierlog.CreatedAtLTE(*filter.DateTo))
	}
	if filter.FromID > 0 {
		q = q.Where(cashierlog.IDLT(filter.FromID))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q = q.Limit(int(limit))

	return q.All(ctx)
}
