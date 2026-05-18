package service

import (
	"context"
	"testing"
	"time"

	v1 "github.com/makesalekz/sales/api/sales/v1"
	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/ent/enum"
	"github.com/makesalekz/sales/internal/biz"
	"github.com/makesalekz/sales/internal/data"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock CashierLogRepo ---

type mockCashierLogRepo struct {
	logs   []*ent.CashierLog
	nextID int64
}

func newMockCashierLogRepo() *mockCashierLogRepo {
	return &mockCashierLogRepo{nextID: 1}
}

func (m *mockCashierLogRepo) Create(_ context.Context, dto data.CashierLogDto) (*ent.CashierLog, error) {
	entry := &ent.CashierLog{
		ID:          m.nextID,
		TenantID:    dto.TenantID,
		CashierID:   dto.CashierID,
		ShiftID:     dto.ShiftID,
		Action:      dto.Action,
		EntityID:    dto.EntityID,
		EntityType:  dto.EntityType,
		Amount:      dto.Amount,
		Description: dto.Description,
		CreatedAt:   time.Now(),
	}
	m.logs = append(m.logs, entry)
	m.nextID++
	return entry, nil
}

func (m *mockCashierLogRepo) List(_ context.Context, filter data.CashierLogFilter) ([]*ent.CashierLog, error) {
	var result []*ent.CashierLog
	limit := int(filter.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	for i := len(m.logs) - 1; i >= 0; i-- {
		entry := m.logs[i]
		if entry.TenantID != filter.TenantID {
			continue
		}
		if filter.CashierID > 0 && entry.CashierID != filter.CashierID {
			continue
		}
		if filter.ShiftID > 0 && entry.ShiftID != filter.ShiftID {
			continue
		}
		if filter.FromID > 0 && entry.ID >= filter.FromID {
			continue
		}
		if filter.DateFrom != nil && entry.CreatedAt.Before(*filter.DateFrom) {
			continue
		}
		if filter.DateTo != nil && entry.CreatedAt.After(*filter.DateTo) {
			continue
		}
		result = append(result, entry)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

// --- Recording CashierLogger (tracks entries for assertions) ---

type recordingCashierLogger struct {
	repo *mockCashierLogRepo
}

func newRecordingCashierLogger(repo *mockCashierLogRepo) biz.CashierLogger {
	return biz.NewCashierLogger(repo, log.DefaultLogger)
}

// --- Test helpers ---

func setupCashierLogService(repo *mockCashierLogRepo) *CashierLogServiceImpl {
	uc := biz.NewCashierLogUsecase(repo)
	return NewCashierLogService(uc)
}

func setupSaleServiceWithRecording(repo *mockCashierLogRepo) (*SalesService, *mockSalesRepo) {
	salesRepo := newMockSalesRepo()
	pub := newMockPublisher()
	logger := newRecordingCashierLogger(repo)
	uc := biz.NewSalesUsecase(log.DefaultLogger, salesRepo, pub, logger)
	svc := NewSalesService(uc)
	return svc, salesRepo
}

func setupShiftServiceWithRecording(repo *mockCashierLogRepo) (*ShiftServiceImpl, *mockShiftsRepo) {
	shiftsRepo := newMockShiftsRepo()
	logger := newRecordingCashierLogger(repo)
	uc := biz.NewShiftsUsecase(shiftsRepo, logger)
	svc := NewShiftService(uc)
	return svc, shiftsRepo
}

func setupReturnServiceWithRecording(repo *mockCashierLogRepo) (*ReturnsServiceImpl, *mockSalesRepo, *mockReturnsRepo) {
	salesRepo := newMockSalesRepo()
	returnsRepo := newMockReturnsRepo()
	pub := newMockReturnPublisher()
	logger := newRecordingCashierLogger(repo)
	uc := biz.NewReturnsUsecase(log.DefaultLogger, returnsRepo, salesRepo, pub, logger)
	svc := NewReturnsService(uc)
	return svc, salesRepo, returnsRepo
}

// --- Tests: Auto-logging from operations ---

func TestCashierLog_SaleCreatesLogEntry(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	svc, _ := setupSaleServiceWithRecording(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		ShiftId:     10,
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "Test", Quantity: "2", UnitPrice: "100"},
		},
	})
	require.NoError(t, err)

	require.Len(t, logRepo.logs, 1)
	entry := logRepo.logs[0]
	assert.Equal(t, int64(1), entry.TenantID)
	assert.Equal(t, int64(42), entry.CashierID)
	assert.Equal(t, int64(10), entry.ShiftID)
	assert.Equal(t, enum.ActionSale, entry.Action)
	assert.Equal(t, resp.Sale.Id, entry.EntityID)
	assert.Equal(t, "sale", entry.EntityType)
	assert.True(t, entry.Amount.Equal(decimal.NewFromInt(200)))
}

func TestCashierLog_ReturnCreatesLogEntry(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	svc, salesRepo, _ := setupReturnServiceWithRecording(logRepo)

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "A", Quantity: decimal.NewFromInt(3), UnitPrice: decimal.NewFromInt(50), Total: decimal.NewFromInt(150)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)
	resp, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "2"},
		},
	})
	require.NoError(t, err)

	require.Len(t, logRepo.logs, 1)
	entry := logRepo.logs[0]
	assert.Equal(t, int64(1), entry.TenantID)
	assert.Equal(t, int64(42), entry.CashierID)
	assert.Equal(t, enum.ActionReturn, entry.Action)
	assert.Equal(t, resp.SaleReturn.Id, entry.EntityID)
	assert.Equal(t, "return", entry.EntityType)
	assert.True(t, entry.Amount.Equal(decimal.NewFromInt(100))) // 2 * 50
}

func TestCashierLog_OpenShiftCreatesLogEntry(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	svc, _ := setupShiftServiceWithRecording(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{
		OpeningAmount: "5000",
	})
	require.NoError(t, err)

	require.Len(t, logRepo.logs, 1)
	entry := logRepo.logs[0]
	assert.Equal(t, int64(1), entry.TenantID)
	assert.Equal(t, int64(42), entry.CashierID)
	assert.Equal(t, resp.Shift.Id, entry.ShiftID)
	assert.Equal(t, enum.ActionShiftOpen, entry.Action)
	assert.Equal(t, resp.Shift.Id, entry.EntityID)
	assert.Equal(t, "shift", entry.EntityType)
	assert.True(t, entry.Amount.Equal(decimal.NewFromInt(5000)))
}

func TestCashierLog_CloseShiftCreatesLogEntry(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	svc, _ := setupShiftServiceWithRecording(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	openResp, err := svc.OpenShift(ctx, &v1.OpenShiftRequest{OpeningAmount: "1000"})
	require.NoError(t, err)

	_, err = svc.CloseShift(ctx, &v1.CloseShiftRequest{ShiftId: openResp.Shift.Id})
	require.NoError(t, err)

	// Two log entries: SHIFT_OPEN + SHIFT_CLOSE
	require.Len(t, logRepo.logs, 2)

	openEntry := logRepo.logs[0]
	assert.Equal(t, enum.ActionShiftOpen, openEntry.Action)

	closeEntry := logRepo.logs[1]
	assert.Equal(t, enum.ActionShiftClose, closeEntry.Action)
	assert.Equal(t, openResp.Shift.Id, closeEntry.ShiftID)
	assert.Equal(t, openResp.Shift.Id, closeEntry.EntityID)
	assert.Equal(t, "shift", closeEntry.EntityType)
}

// --- Tests: GetCashierLog RPC ---

func TestGetCashierLog_HappyPath(t *testing.T) {
	logRepo := newMockCashierLogRepo()

	// Seed some log entries
	now := time.Now()
	logRepo.logs = append(logRepo.logs,
		&ent.CashierLog{ID: 1, TenantID: 1, CashierID: 42, ShiftID: 10, Action: enum.ActionSale, EntityID: 100, EntityType: "sale", Amount: decimal.NewFromInt(500), CreatedAt: now},
		&ent.CashierLog{ID: 2, TenantID: 1, CashierID: 42, ShiftID: 10, Action: enum.ActionReturn, EntityID: 200, EntityType: "return", Amount: decimal.NewFromInt(100), CreatedAt: now},
	)
	logRepo.nextID = 3

	svc := setupCashierLogService(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{
		CashierId: 42,
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Entries, 2)
}

func TestGetCashierLog_FilterByCashier(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	now := time.Now()
	logRepo.logs = append(logRepo.logs,
		&ent.CashierLog{ID: 1, TenantID: 1, CashierID: 42, Action: enum.ActionSale, EntityID: 1, EntityType: "sale", Amount: decimal.Zero, CreatedAt: now},
		&ent.CashierLog{ID: 2, TenantID: 1, CashierID: 99, Action: enum.ActionSale, EntityID: 2, EntityType: "sale", Amount: decimal.Zero, CreatedAt: now},
	)
	logRepo.nextID = 3

	svc := setupCashierLogService(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{CashierId: 42, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Entries, 1)
	assert.Equal(t, int64(42), resp.Entries[0].CashierId)
}

func TestGetCashierLog_FilterByShift(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	now := time.Now()
	logRepo.logs = append(logRepo.logs,
		&ent.CashierLog{ID: 1, TenantID: 1, CashierID: 42, ShiftID: 10, Action: enum.ActionSale, EntityID: 1, EntityType: "sale", Amount: decimal.Zero, CreatedAt: now},
		&ent.CashierLog{ID: 2, TenantID: 1, CashierID: 42, ShiftID: 20, Action: enum.ActionSale, EntityID: 2, EntityType: "sale", Amount: decimal.Zero, CreatedAt: now},
	)
	logRepo.nextID = 3

	svc := setupCashierLogService(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{ShiftId: 10, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Entries, 1)
	assert.Equal(t, int64(10), resp.Entries[0].ShiftId)
}

func TestGetCashierLog_TenantIsolation(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	now := time.Now()
	logRepo.logs = append(logRepo.logs,
		&ent.CashierLog{ID: 1, TenantID: 1, CashierID: 42, Action: enum.ActionSale, EntityID: 1, EntityType: "sale", Amount: decimal.Zero, CreatedAt: now},
		&ent.CashierLog{ID: 2, TenantID: 2, CashierID: 42, Action: enum.ActionSale, EntityID: 2, EntityType: "sale", Amount: decimal.Zero, CreatedAt: now},
	)
	logRepo.nextID = 3

	svc := setupCashierLogService(logRepo)

	// Tenant 1 sees only their logs
	ctx1 := ctxWithTenantAndActor(1, 42)
	resp1, err := svc.GetCashierLog(ctx1, &v1.GetCashierLogRequest{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp1.Entries, 1)
	assert.Equal(t, int64(1), resp1.Entries[0].TenantId)

	// Tenant 2 sees only their logs
	ctx2 := ctxWithTenantAndActor(2, 42)
	resp2, err := svc.GetCashierLog(ctx2, &v1.GetCashierLogRequest{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp2.Entries, 1)
	assert.Equal(t, int64(2), resp2.Entries[0].TenantId)
}

func TestGetCashierLog_NoTenant(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	svc := setupCashierLogService(logRepo)
	ctx := context.Background()

	_, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{Limit: 10})
	require.Error(t, err)
}

func TestGetCashierLog_Pagination(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	now := time.Now()
	for i := int64(1); i <= 5; i++ {
		logRepo.logs = append(logRepo.logs, &ent.CashierLog{
			ID: i, TenantID: 1, CashierID: 42, Action: enum.ActionSale,
			EntityID: i, EntityType: "sale", Amount: decimal.Zero, CreatedAt: now,
		})
	}
	logRepo.nextID = 6

	svc := setupCashierLogService(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	// First page: get 2 entries (newest first)
	resp1, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, resp1.Entries, 2)
	assert.Equal(t, int64(5), resp1.Entries[0].Id)
	assert.Equal(t, int64(4), resp1.Entries[1].Id)

	// Second page: from_id = 4 (exclusive)
	resp2, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{Limit: 2, FromId: 4})
	require.NoError(t, err)
	assert.Len(t, resp2.Entries, 2)
	assert.Equal(t, int64(3), resp2.Entries[0].Id)
	assert.Equal(t, int64(2), resp2.Entries[1].Id)
}

func TestGetCashierLog_InvalidDateFormat(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	svc := setupCashierLogService(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{DateFrom: "not-a-date"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid date_from")

	_, err = svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{DateTo: "not-a-date"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid date_to")
}

func TestGetCashierLog_EntryFieldsMapping(t *testing.T) {
	logRepo := newMockCashierLogRepo()
	now := time.Now()
	logRepo.logs = append(logRepo.logs, &ent.CashierLog{
		ID:          1,
		TenantID:    1,
		CashierID:   42,
		ShiftID:     10,
		Action:      enum.ActionShiftOpen,
		EntityID:    10,
		EntityType:  "shift",
		Amount:      decimal.NewFromInt(5000),
		Description: "Shift #10 opened",
		CreatedAt:   now,
	})
	logRepo.nextID = 2

	svc := setupCashierLogService(logRepo)
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.GetCashierLog(ctx, &v1.GetCashierLogRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Entries, 1)

	e := resp.Entries[0]
	assert.Equal(t, int64(1), e.Id)
	assert.Equal(t, int64(1), e.TenantId)
	assert.Equal(t, int64(42), e.CashierId)
	assert.Equal(t, int64(10), e.ShiftId)
	assert.Equal(t, "SHIFT_OPEN", e.Action)
	assert.Equal(t, int64(10), e.EntityId)
	assert.Equal(t, "shift", e.EntityType)
	assert.Equal(t, "5000", e.Amount)
	assert.Equal(t, "Shift #10 opened", e.Description)
	assert.NotEmpty(t, e.CreatedAt)
}

// --- Tests: Auto-logging does NOT fail parent operation ---

func TestCashierLog_SaleSucceedsEvenIfLogFails(t *testing.T) {
	// Use a failing log repo
	failRepo := &failingCashierLogRepo{}
	logger := biz.NewCashierLogger(failRepo, log.DefaultLogger)

	salesRepo := newMockSalesRepo()
	pub := newMockPublisher()
	uc := biz.NewSalesUsecase(log.DefaultLogger, salesRepo, pub, logger)
	svc := NewSalesService(uc)

	ctx := ctxWithTenantAndActor(1, 42)
	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "100", resp.Sale.Total)
}

// --- Failing CashierLogRepo ---

type failingCashierLogRepo struct{}

func (r *failingCashierLogRepo) Create(_ context.Context, _ data.CashierLogDto) (*ent.CashierLog, error) {
	return nil, assert.AnError
}

func (r *failingCashierLogRepo) List(_ context.Context, _ data.CashierLogFilter) ([]*ent.CashierLog, error) {
	return nil, assert.AnError
}
