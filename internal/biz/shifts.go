package biz

import (
	"context"
	"fmt"

	"gitlab.calendaria.team/services/sales/ent"
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/internal/data"

	"github.com/shopspring/decimal"
)

type ShiftsUsecase struct {
	repo       data.ShiftsRepo
	cashierLog CashierLogger
}

func NewShiftsUsecase(repo data.ShiftsRepo, cashierLog CashierLogger) *ShiftsUsecase {
	return &ShiftsUsecase{repo: repo, cashierLog: cashierLog}
}

type OpenShiftInput struct {
	TenantID      int64
	CashierID     int64
	OpeningAmount decimal.Decimal
	StoreID       *int64
}

type CloseShiftResult struct {
	Shift   *ent.Shift
	Summary *data.ShiftSummary
}

func (uc *ShiftsUsecase) OpenShift(ctx context.Context, input OpenShiftInput) (*ent.Shift, error) {
	// Check if there's already an open shift for this cashier.
	// GetOpenByTenantAndCashier returns (nil, nil) if no open shift exists,
	// or (nil, error) on DB failure.
	existing, err := uc.repo.GetOpenByTenantAndCashier(ctx, input.TenantID, input.CashierID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing shifts: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("cashier already has an open shift (id=%d)", existing.ID)
	}

	dto := data.ShiftDto{
		TenantID:      input.TenantID,
		CashierID:     input.CashierID,
		OpeningAmount: input.OpeningAmount,
		StoreID:       input.StoreID,
	}

	shift, err := uc.repo.Create(ctx, dto)
	if err != nil {
		return nil, err
	}

	// Log cashier action (best-effort)
	uc.cashierLog.Log(ctx, CashierLogEntry{
		TenantID:    input.TenantID,
		CashierID:   input.CashierID,
		ShiftID:     shift.ID,
		Action:      enum.ActionShiftOpen,
		EntityID:    shift.ID,
		EntityType:  "shift",
		Amount:      input.OpeningAmount,
		Description: fmt.Sprintf("Shift #%d opened", shift.ID),
	})

	return shift, nil
}

func (uc *ShiftsUsecase) CloseShift(ctx context.Context, shiftID, tenantID, cashierID int64) (*CloseShiftResult, error) {
	s, err := uc.repo.GetByID(ctx, shiftID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("shift not found")
	}

	if s.Status != enum.ShiftOpen {
		return nil, fmt.Errorf("shift is already closed")
	}

	if s.CashierID != cashierID {
		return nil, fmt.Errorf("shift does not belong to this cashier")
	}

	// Calculate totals from sales in this shift
	summary, err := uc.repo.GetShiftSalesSummary(ctx, shiftID)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate shift summary: %w", err)
	}

	// closing_amount = opening_amount + total_sales - total_returns
	closingAmount := s.OpeningAmount.Add(summary.TotalSales).Sub(summary.TotalReturns)

	updatedShift, err := uc.repo.Close(ctx, shiftID, closingAmount, summary.TotalSales, summary.TotalReturns)
	if err != nil {
		return nil, fmt.Errorf("failed to close shift: %w", err)
	}

	// Log cashier action (best-effort)
	uc.cashierLog.Log(ctx, CashierLogEntry{
		TenantID:    tenantID,
		CashierID:   cashierID,
		ShiftID:     shiftID,
		Action:      enum.ActionShiftClose,
		EntityID:    shiftID,
		EntityType:  "shift",
		Amount:      closingAmount,
		Description: fmt.Sprintf("Shift #%d closed", shiftID),
	})

	return &CloseShiftResult{
		Shift:   updatedShift,
		Summary: summary,
	}, nil
}

func (uc *ShiftsUsecase) GetShift(ctx context.Context, shiftID, tenantID int64) (*ent.Shift, error) {
	s, err := uc.repo.GetByID(ctx, shiftID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("shift not found")
	}
	return s, nil
}

func (uc *ShiftsUsecase) ListShifts(ctx context.Context, tenantID int64, storeID *int64, limit int32, fromID int64) ([]*ent.Shift, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return uc.repo.List(ctx, tenantID, storeID, limit, fromID)
}
