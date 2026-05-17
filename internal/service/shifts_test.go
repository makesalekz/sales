package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "gitlab.calendaria.team/services/sales/api/sales/v1"
	"gitlab.calendaria.team/services/sales/ent"
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/internal/biz"
	"gitlab.calendaria.team/services/sales/internal/data"
	"gitlab.calendaria.team/services/utils/v2/auth"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock ShiftsRepo ---

type mockShiftsRepo struct {
	shifts map[int64]*ent.Shift
	sales  []*ent.Sale
	nextID int64
}

func newMockShiftsRepo() *mockShiftsRepo {
	return &mockShiftsRepo{
		shifts: make(map[int64]*ent.Shift),
		nextID: 1,
	}
}

func (m *mockShiftsRepo) Create(_ context.Context, dto data.ShiftDto) (*ent.Shift, error) {
	s := &ent.Shift{
		ID:            m.nextID,
		TenantID:      dto.TenantID,
		CashierID:     dto.CashierID,
		OpeningAmount: dto.OpeningAmount,
		Status:        enum.ShiftOpen,
		OpenedAt:      time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	m.shifts[m.nextID] = s
	m.nextID++
	return s, nil
}

func (m *mockShiftsRepo) GetByID(_ context.Context, id, tenantID int64) (*ent.Shift, error) {
	s, ok := m.shifts[id]
	if !ok || s.TenantID != tenantID {
		return nil, fmt.Errorf("not found")
	}
	return s, nil
}

func (m *mockShiftsRepo) GetOpenByTenantAndCashier(_ context.Context, tenantID, cashierID int64) (*ent.Shift, error) {
	for _, s := range m.shifts {
		if s.TenantID == tenantID && s.CashierID == cashierID && s.Status == enum.ShiftOpen {
			return s, nil
		}
	}
	// Contract: return (nil, nil) when no open shift found
	return nil, nil
}

func (m *mockShiftsRepo) Close(_ context.Context, id int64, closingAmount, totalSales, totalReturns decimal.Decimal) (*ent.Shift, error) {
	s, ok := m.shifts[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	now := time.Now()
	s.Status = enum.ShiftClosed
	s.ClosingAmount = closingAmount
	s.TotalSales = totalSales
	s.TotalReturns = totalReturns
	s.ClosedAt = &now
	return s, nil
}

func (m *mockShiftsRepo) List(_ context.Context, tenantID int64, _ *int64, limit int32, fromID int64) ([]*ent.Shift, error) {
	var result []*ent.Shift
	for _, s := range m.shifts {
		if s.TenantID != tenantID {
			continue
		}
		if fromID > 0 && s.ID >= fromID {
			continue
		}
		result = append(result, s)
		if int32(len(result)) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockShiftsRepo) GetShiftSalesSummary(_ context.Context, shiftID int64) (*data.ShiftSummary, error) {
	var totalSales, totalReturns decimal.Decimal
	var salesCount, returnsCount int32
	for _, sale := range m.sales {
		if sale.ShiftID != shiftID {
			continue
		}
		if sale.Status == enum.Completed {
			totalSales = totalSales.Add(sale.Total)
			salesCount++
		} else if sale.Status == enum.Returned {
			totalReturns = totalReturns.Add(sale.Total)
			returnsCount++
		}
	}
	return &data.ShiftSummary{
		TotalSales:   totalSales,
		TotalReturns: totalReturns,
		SalesCount:   salesCount,
		ReturnsCount: returnsCount,
	}, nil
}

// --- Test setup ---

func setupShiftService() (*ShiftServiceImpl, *mockShiftsRepo) {
	repo := newMockShiftsRepo()
	uc := biz.NewShiftsUsecase(repo, noopCashierLogger{})
	svc := NewShiftService(uc)
	return svc, repo
}

// --- Tests ---

func TestOpenShift_HappyPath(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{
		OpeningAmount: "5000",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Shift)

	assert.Equal(t, int64(1), resp.Shift.TenantId)
	assert.Equal(t, int64(42), resp.Shift.CashierId)
	assert.Equal(t, "5000", resp.Shift.OpeningAmount)
	assert.Equal(t, "OPEN", resp.Shift.Status)
	assert.NotEmpty(t, resp.Shift.OpenedAt)
}

func TestOpenShift_AlreadyOpen(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	// Second open should fail
	_, err = svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "2000"})
	require.Error(t, err)
}

func TestOpenShift_DifferentCashierCanOpen(t *testing.T) {
	svc, _ := setupShiftService()
	ctx1 := ctxWithTenantAndActor(1, 42)
	ctx2 := ctxWithTenantAndActor(1, 43)

	_, err := svc.OpenShift(ctx1, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	// Different cashier should succeed
	resp, err := svc.OpenShift(ctx2, &v1.OpenShiftRequest{OpeningAmount: "2000"})
	require.NoError(t, err)
	assert.Equal(t, int64(43), resp.Shift.CashierId)
}

func TestOpenShift_NoTenant(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := context.Background()

	_, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.Error(t, err)
}

func TestOpenShift_NoActor(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := auth.NewTenantContext(context.Background(), 1)

	_, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.Error(t, err)
}

func TestCloseShift_HappyPath(t *testing.T) {
	svc, repo := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Open a shift
	openResp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "5000"})
	require.NoError(t, err)
	shiftID := openResp.Shift.Id

	// Add some sales to the repo
	repo.sales = append(repo.sales,
		&ent.Sale{ID: 1, ShiftID: shiftID, Total: decimal.NewFromInt(1500), Status: enum.Completed},
		&ent.Sale{ID: 2, ShiftID: shiftID, Total: decimal.NewFromInt(800), Status: enum.Completed},
		&ent.Sale{ID: 3, ShiftID: shiftID, Total: decimal.NewFromInt(200), Status: enum.Returned},
	)

	// Close the shift
	closeResp, err := svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: shiftID})
	require.NoError(t, err)
	require.NotNil(t, closeResp.Shift)
	require.NotNil(t, closeResp.ZReport)

	assert.Equal(t, "CLOSED", closeResp.Shift.Status)
	assert.Equal(t, "2300", closeResp.Shift.TotalSales)
	assert.Equal(t, "200", closeResp.Shift.TotalReturns)
	// closing = opening + sales - returns = 5000 + 2300 - 200 = 7100
	assert.Equal(t, "7100", closeResp.Shift.ClosingAmount)
	assert.NotEmpty(t, closeResp.Shift.ClosedAt)

	// Z-Report verification
	assert.Equal(t, shiftID, closeResp.ZReport.ShiftId)
	assert.Equal(t, int64(42), closeResp.ZReport.CashierId)
	assert.Equal(t, "5000", closeResp.ZReport.OpeningAmount)
	assert.Equal(t, "7100", closeResp.ZReport.ClosingAmount)
	assert.Equal(t, "2300", closeResp.ZReport.TotalSales)
	assert.Equal(t, "200", closeResp.ZReport.TotalReturns)
	assert.Equal(t, "7100", closeResp.ZReport.ExpectedCash)
	assert.Equal(t, int32(2), closeResp.ZReport.SalesCount)
	assert.Equal(t, int32(1), closeResp.ZReport.ReturnsCount)
}

func TestCloseShift_NoSales(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	openResp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "3000"})
	require.NoError(t, err)

	closeResp, err := svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: openResp.Shift.Id})
	require.NoError(t, err)

	assert.Equal(t, "CLOSED", closeResp.Shift.Status)
	assert.Equal(t, "0", closeResp.Shift.TotalSales)
	assert.Equal(t, "0", closeResp.Shift.TotalReturns)
	assert.Equal(t, "3000", closeResp.Shift.ClosingAmount)
}

func TestCloseShift_AlreadyClosed(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	openResp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	_, err = svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: openResp.Shift.Id})
	require.NoError(t, err)

	// Close again should fail
	_, err = svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: openResp.Shift.Id})
	require.Error(t, err)
}

func TestCloseShift_WrongCashier(t *testing.T) {
	svc, _ := setupShiftService()
	ctx1 := ctxWithTenantAndActor(1, 42)
	ctx2 := ctxWithTenantAndActor(1, 99)

	openResp, err := svc.OpenShift(ctx1, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	// Different cashier tries to close
	_, err = svc.CloseShift(ctx2, &v1.CloseShiftRequest{ShiftId: openResp.Shift.Id})
	require.Error(t, err)
}

func TestCloseShift_NotFound(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: 999})
	require.Error(t, err)
}

func TestGetShift_HappyPath(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	openResp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "2500"})
	require.NoError(t, err)

	getResp, err := svc.GetShift(ctx, &v1.GetShiftRequest{ShiftId: openResp.Shift.Id})
	require.NoError(t, err)
	assert.Equal(t, openResp.Shift.Id, getResp.Shift.Id)
	assert.Equal(t, "2500", getResp.Shift.OpeningAmount)
	assert.Equal(t, "OPEN", getResp.Shift.Status)
}

func TestGetShift_NotFound(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.GetShift(ctx, &v1.GetShiftRequest{ShiftId: 999})
	require.Error(t, err)
}

func TestGetShift_WrongTenant(t *testing.T) {
	svc, _ := setupShiftService()
	ctx1 := ctxWithTenantAndActor(1, 42)
	ctx2 := ctxWithTenantAndActor(2, 42)

	openResp, err := svc.OpenShift(ctx1, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	// Different tenant can't access
	_, err = svc.GetShift(ctx2, &v1.GetShiftRequest{ShiftId: openResp.Shift.Id})
	require.Error(t, err)
}

func TestListShifts_HappyPath(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Create multiple shifts (close first ones to allow new ones for same cashier)
	_, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	// Use different cashier for second shift
	ctx2 := ctxWithTenantAndActor(1, 43)
	_, err = svc.OpenShift(ctx2, &v1.OpenShiftRequest{OpeningAmount: "2000"})
	require.NoError(t, err)

	listResp, err := svc.ListShifts(ctx, &v1.ListShiftsRequest{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, listResp.Shifts, 2)
}

func TestListShifts_TenantIsolation(t *testing.T) {
	svc, _ := setupShiftService()
	ctx1 := ctxWithTenantAndActor(1, 42)
	ctx2 := ctxWithTenantAndActor(2, 43)

	_, err := svc.OpenShift(ctx1, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	_, err = svc.OpenShift(ctx2, &v1.OpenShiftRequest{OpeningAmount: "2000"})
	require.NoError(t, err)

	listResp, err := svc.ListShifts(ctx1, &v1.ListShiftsRequest{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, listResp.Shifts, 1)
	assert.Equal(t, int64(1), listResp.Shifts[0].TenantId)
}

func TestListShifts_NoTenant(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := context.Background()

	_, err := svc.ListShifts(ctx, &v1.ListShiftsRequest{Limit: 10})
	require.Error(t, err)
}

func TestOpenShift_ZeroOpeningAmount(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "0"})
	require.NoError(t, err)
	assert.Equal(t, "0", resp.Shift.OpeningAmount)
}

func TestCloseShift_ZReportTimestamps(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	openResp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	closeResp, err := svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: openResp.Shift.Id})
	require.NoError(t, err)

	assert.NotEmpty(t, closeResp.ZReport.OpenedAt)
	assert.NotEmpty(t, closeResp.ZReport.ClosedAt)
}

func TestOpenShift_CanReopenAfterClose(t *testing.T) {
	svc, _ := setupShiftService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Open shift
	openResp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	// Close shift
	_, err = svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: openResp.Shift.Id})
	require.NoError(t, err)

	// Open new shift should succeed
	resp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "2000"})
	require.NoError(t, err)
	assert.Equal(t, "OPEN", resp.Shift.Status)
	assert.Equal(t, "2000", resp.Shift.OpeningAmount)
}
