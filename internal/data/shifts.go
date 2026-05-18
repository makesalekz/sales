package data

import (
	"context"
	"time"

	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/ent/enum"
	"github.com/makesalekz/sales/ent/sale"
	"github.com/makesalekz/sales/ent/shift"

	"github.com/shopspring/decimal"
)

type ShiftDto struct {
	TenantID      int64
	CashierID     int64
	OpeningAmount decimal.Decimal
	StoreID       *int64
}

type ShiftSummary struct {
	TotalSales   decimal.Decimal
	TotalReturns decimal.Decimal
	SalesCount   int32
	ReturnsCount int32
}

type ShiftsRepo interface {
	Create(ctx context.Context, dto ShiftDto) (*ent.Shift, error)
	GetByID(ctx context.Context, id, tenantID int64) (*ent.Shift, error)
	GetOpenByTenantAndCashier(ctx context.Context, tenantID, cashierID int64) (*ent.Shift, error)
	Close(ctx context.Context, id int64, closingAmount, totalSales, totalReturns decimal.Decimal) (*ent.Shift, error)
	List(ctx context.Context, tenantID int64, storeID *int64, limit int32, fromID int64) ([]*ent.Shift, error)
	GetShiftSalesSummary(ctx context.Context, shiftID int64) (*ShiftSummary, error)
}

type shiftsRepo struct {
	db *ent.Client
}

func NewShiftsRepo(d *Data) ShiftsRepo {
	return &shiftsRepo{db: d.db}
}

func (r *shiftsRepo) Create(ctx context.Context, dto ShiftDto) (*ent.Shift, error) {
	create := r.db.Shift.Create().
		SetTenantID(dto.TenantID).
		SetCashierID(dto.CashierID).
		SetOpeningAmount(dto.OpeningAmount).
		SetStatus(enum.ShiftOpen)
	if dto.StoreID != nil {
		create.SetStoreID(*dto.StoreID)
	}
	return create.Save(ctx)
}

func (r *shiftsRepo) GetByID(ctx context.Context, id, tenantID int64) (*ent.Shift, error) {
	return r.db.Shift.Query().
		Where(shift.ID(id), shift.TenantID(tenantID)).
		Only(ctx)
}

func (r *shiftsRepo) GetOpenByTenantAndCashier(ctx context.Context, tenantID, cashierID int64) (*ent.Shift, error) {
	s, err := r.db.Shift.Query().
		Where(
			shift.TenantID(tenantID),
			shift.CashierID(cashierID),
			shift.StatusEQ(enum.ShiftOpen),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return s, err
}

func (r *shiftsRepo) Close(ctx context.Context, id int64, closingAmount, totalSales, totalReturns decimal.Decimal) (*ent.Shift, error) {
	now := time.Now()
	return r.db.Shift.UpdateOneID(id).
		SetStatus(enum.ShiftClosed).
		SetClosingAmount(closingAmount).
		SetTotalSales(totalSales).
		SetTotalReturns(totalReturns).
		SetClosedAt(now).
		Save(ctx)
}

func (r *shiftsRepo) List(ctx context.Context, tenantID int64, storeID *int64, limit int32, fromID int64) ([]*ent.Shift, error) {
	q := r.db.Shift.Query().
		Where(shift.TenantID(tenantID)).
		Order(ent.Desc(shift.FieldID)).
		Limit(int(limit))

	if storeID != nil {
		q = q.Where(shift.StoreID(*storeID))
	}

	if fromID > 0 {
		q = q.Where(shift.IDLT(fromID))
	}

	return q.All(ctx)
}

func (r *shiftsRepo) GetShiftSalesSummary(ctx context.Context, shiftID int64) (*ShiftSummary, error) {
	// Get completed sales totals
	sales, err := r.db.Sale.Query().
		Where(sale.ShiftID(shiftID), sale.StatusEQ(enum.Completed)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	var totalSales decimal.Decimal
	for _, s := range sales {
		totalSales = totalSales.Add(s.Total)
	}

	// Get returned sales totals
	returns, err := r.db.Sale.Query().
		Where(sale.ShiftID(shiftID), sale.StatusEQ(enum.Returned)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	var totalReturns decimal.Decimal
	for _, s := range returns {
		totalReturns = totalReturns.Add(s.Total)
	}

	return &ShiftSummary{
		TotalSales:   totalSales,
		TotalReturns: totalReturns,
		SalesCount:   int32(len(sales)),
		ReturnsCount: int32(len(returns)),
	}, nil
}
