package service

import (
	"context"
	"testing"
	"time"

	v1 "gitlab.calendaria.team/services/sales/api/sales/v1"
	"gitlab.calendaria.team/services/sales/ent"
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/internal/biz"
	"gitlab.calendaria.team/services/sales/internal/data"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock ReturnsRepo ---

type mockReturnsRepo struct {
	returns  map[int64]*ent.SaleReturn
	items    map[int64][]*ent.SaleReturnItem
	nextID   int64
	itemID   int64
}

func newMockReturnsRepo() *mockReturnsRepo {
	return &mockReturnsRepo{
		returns: make(map[int64]*ent.SaleReturn),
		items:   make(map[int64][]*ent.SaleReturnItem),
		nextID:  1,
		itemID:  1,
	}
}

func (m *mockReturnsRepo) Create(_ context.Context, dto data.ReturnDto) (*ent.SaleReturn, error) {
	var uuid *string
	if dto.UUID != "" {
		uuid = &dto.UUID
	}
	sr := &ent.SaleReturn{
		ID:        m.nextID,
		UUID:      uuid,
		TenantID:  dto.TenantID,
		SaleID:    dto.SaleID,
		CashierID: dto.CashierID,
		Total:     dto.Total,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.returns[m.nextID] = sr

	var returnItems []*ent.SaleReturnItem
	for _, item := range dto.Items {
		ri := &ent.SaleReturnItem{
			ID:           m.itemID,
			SaleReturnID: sr.ID,
			SaleItemID:   item.SaleItemID,
			ProductID:    item.ProductID,
			Quantity:     item.Quantity,
			UnitPrice:    item.UnitPrice,
			Total:        item.Total,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		returnItems = append(returnItems, ri)
		m.itemID++
	}
	m.items[sr.ID] = returnItems
	sr.Edges.Items = returnItems

	m.nextID++
	return sr, nil
}

func (m *mockReturnsRepo) GetReturnedQuantities(_ context.Context, saleID int64) (map[int64]decimal.Decimal, error) {
	result := make(map[int64]decimal.Decimal)
	for _, sr := range m.returns {
		if sr.SaleID != saleID {
			continue
		}
		for _, item := range m.items[sr.ID] {
			result[item.SaleItemID] = result[item.SaleItemID].Add(item.Quantity)
		}
	}
	return result, nil
}

func (m *mockReturnsRepo) GetByUUID(_ context.Context, uuid string) (*ent.SaleReturn, error) {
	for _, sr := range m.returns {
		if sr.UUID != nil && *sr.UUID == uuid {
			sr.Edges.Items = m.items[sr.ID]
			return sr, nil
		}
	}
	return nil, assert.AnError
}

// --- Mock Return Publisher ---

type mockReturnPublisher struct {
	events []data.ReturnCompletedEvent
}

func newMockReturnPublisher() *mockReturnPublisher {
	return &mockReturnPublisher{}
}

func (p *mockReturnPublisher) Publish(event data.ReturnCompletedEvent) {
	p.events = append(p.events, event)
}

// --- Test setup ---

// createSaleInRepo creates a sale directly in mockSalesRepo and returns the sale ID.
func createSaleInRepo(repo *mockSalesRepo, tenantID, cashierID int64, items []data.SaleItemDto) int64 {
	var total decimal.Decimal
	for _, item := range items {
		total = total.Add(item.Total)
	}

	s := &ent.Sale{
		ID:        repo.nextID,
		TenantID:  tenantID,
		CashierID: cashierID,
		Total:     total,
		Status:    enum.Completed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo.sales[repo.nextID] = s

	var saleItems []*ent.SaleItem
	for _, item := range items {
		si := &ent.SaleItem{
			ID:          repo.itemID,
			SaleID:      s.ID,
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Discount:    item.Discount,
			Total:       item.Total,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		saleItems = append(saleItems, si)
		repo.itemID++
	}
	repo.items[s.ID] = saleItems
	s.Edges.Items = saleItems

	repo.nextID++
	return s.ID
}

func setupReturnService() (*ReturnsServiceImpl, *mockSalesRepo, *mockReturnsRepo, *mockReturnPublisher) {
	salesRepo := newMockSalesRepo()
	returnsRepo := newMockReturnsRepo()
	pub := newMockReturnPublisher()
	uc := biz.NewReturnsUsecase(log.DefaultLogger, returnsRepo, salesRepo, pub, noopCashierLogger{})
	svc := NewReturnsService(uc)
	return svc, salesRepo, returnsRepo, pub
}

// --- Tests ---

func TestCreateReturn_FullReturn(t *testing.T) {
	svc, salesRepo, _, pub := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "Молоко", Quantity: decimal.NewFromInt(2), UnitPrice: decimal.NewFromInt(150), Total: decimal.NewFromInt(300)},
		{ProductID: 200, ProductName: "Хлеб", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(80), Total: decimal.NewFromInt(80)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)
	resp, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "2"},
			{SaleItemId: saleItems[1].ID, Quantity: "1"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.SaleReturn)

	assert.Equal(t, int64(1), resp.SaleReturn.TenantId)
	assert.Equal(t, saleID, resp.SaleReturn.SaleId)
	assert.Equal(t, int64(42), resp.SaleReturn.CashierId)
	// total = 2*150 + 1*80 = 380
	assert.Equal(t, "380", resp.SaleReturn.Total)
	assert.Len(t, resp.SaleReturn.Items, 2)
	assert.NotEmpty(t, resp.SaleReturn.CreatedAt)

	// Sale should be marked RETURNED
	assert.Equal(t, enum.Returned, salesRepo.sales[saleID].Status)

	// NATS event with negative total
	require.Len(t, pub.events, 1)
	event := pub.events[0]
	assert.Equal(t, int64(1), event.TenantID)
	assert.Equal(t, saleID, event.SaleID)
	assert.Equal(t, "-380", event.Total)
	assert.NotEmpty(t, event.Timestamp)
}

func TestCreateReturn_PartialReturn(t *testing.T) {
	svc, salesRepo, _, pub := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "Молоко", Quantity: decimal.NewFromInt(5), UnitPrice: decimal.NewFromInt(150), Total: decimal.NewFromInt(750)},
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

	// total = 2*150 = 300
	assert.Equal(t, "300", resp.SaleReturn.Total)
	assert.Len(t, resp.SaleReturn.Items, 1)

	// Sale should stay COMPLETED (partial return)
	assert.Equal(t, enum.Completed, salesRepo.sales[saleID].Status)

	// Event has negative total
	require.Len(t, pub.events, 1)
	assert.Equal(t, "-300", pub.events[0].Total)
}

func TestCreateReturn_TwoPartialReturnsSumToFull(t *testing.T) {
	svc, salesRepo, _, pub := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "Молоко", Quantity: decimal.NewFromInt(4), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(400)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)

	// First partial return: 2 of 4
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "2"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, enum.Completed, salesRepo.sales[saleID].Status)

	// Second partial return: remaining 2 of 4
	_, err = svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "2"},
		},
	})
	require.NoError(t, err)

	// Now sale should be RETURNED
	assert.Equal(t, enum.Returned, salesRepo.sales[saleID].Status)

	// Two events published
	require.Len(t, pub.events, 2)
	assert.Equal(t, "-200", pub.events[0].Total)
	assert.Equal(t, "-200", pub.events[1].Total)
}

func TestCreateReturn_OverReturnRejected(t *testing.T) {
	svc, salesRepo, _, _ := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "Молоко", Quantity: decimal.NewFromInt(2), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(200)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "3"}, // 3 > 2 sold
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds remaining")
}

func TestCreateReturn_AlreadyFullyReturned(t *testing.T) {
	svc, salesRepo, _, _ := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "Молоко", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(100)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)

	// Full return
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "1"},
		},
	})
	require.NoError(t, err)

	// Attempt to return again should fail
	_, err = svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "1"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already fully returned")
}

func TestCreateReturn_SaleItemBelongsToDifferentSale(t *testing.T) {
	svc, salesRepo, _, _ := setupReturnService()

	// Sale 1
	createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "A", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(100)},
	})
	// Sale 2
	sale2ID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 200, ProductName: "B", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(200), Total: decimal.NewFromInt(200)},
	})

	// Try to return sale1's item via sale2
	sale1Items := salesRepo.items[1]
	ctx := ctxWithTenantAndActor(1, 42)
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: sale2ID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: sale1Items[0].ID, Quantity: "1"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong to sale")
}

func TestCreateReturn_TenantIsolation(t *testing.T) {
	svc, salesRepo, _, _ := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "A", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(100)},
	})
	saleItems := salesRepo.items[saleID]

	// Tenant 2 tries to return tenant 1's sale
	ctx := ctxWithTenantAndActor(2, 99)
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "1"},
		},
	})
	require.Error(t, err)
}

func TestCreateReturn_NoTenant(t *testing.T) {
	svc, _, _, _ := setupReturnService()
	ctx := context.Background()

	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: 1,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: 1, Quantity: "1"},
		},
	})
	require.Error(t, err)
}

func TestCreateReturn_EmptyItems(t *testing.T) {
	svc, _, _, _ := setupReturnService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: 1,
		Items:  []*v1.CreateReturnItemRequest{},
	})
	require.Error(t, err)
}

func TestCreateReturn_ZeroQuantityRejected(t *testing.T) {
	svc, salesRepo, _, _ := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "A", Quantity: decimal.NewFromInt(2), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(200)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "0"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "greater than zero")
}

func TestCreateReturn_SaleNotFound(t *testing.T) {
	svc, _, _, _ := setupReturnService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: 999,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: 1, Quantity: "1"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sale not found")
}

func TestCreateReturn_DecimalPrecision(t *testing.T) {
	svc, salesRepo, _, _ := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "A", Quantity: decimal.RequireFromString("2.5"), UnitPrice: decimal.RequireFromString("99.99"), Total: decimal.RequireFromString("249.975")},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)
	resp, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "1.5"},
		},
	})
	require.NoError(t, err)

	// 1.5 * 99.99 = 149.985
	assert.Equal(t, "149.985", resp.SaleReturn.Total)
}

func TestCreateReturn_EventItemFields(t *testing.T) {
	svc, salesRepo, _, pub := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 5, 10, []data.SaleItemDto{
		{ProductID: 42, ProductName: "Чай", Quantity: decimal.NewFromInt(3), UnitPrice: decimal.NewFromInt(50), Total: decimal.NewFromInt(150)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(5, 10)
	resp, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "2"},
		},
	})
	require.NoError(t, err)

	require.Len(t, pub.events, 1)
	event := pub.events[0]
	assert.Equal(t, int64(5), event.TenantID)
	assert.Equal(t, saleID, event.SaleID)
	assert.Equal(t, resp.SaleReturn.Id, event.ReturnID)
	assert.Equal(t, "-100", event.Total) // 2*50 = 100, negated
	require.Len(t, event.Items, 1)
	assert.Equal(t, int64(42), event.Items[0].ProductID)
	assert.Equal(t, saleItems[0].ID, event.Items[0].SaleItemID)
	assert.Equal(t, "2", event.Items[0].Quantity)
	assert.Equal(t, "50", event.Items[0].UnitPrice)
	assert.Equal(t, "-100", event.Items[0].Total)
}

func TestCreateReturn_RepoError_NoEventPublished(t *testing.T) {
	salesRepo := newMockSalesRepo()
	pub := newMockReturnPublisher()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "A", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(100)},
	})

	// Use a failing returns repo
	failRepo := &failingReturnsRepo{}
	uc := biz.NewReturnsUsecase(log.DefaultLogger, failRepo, salesRepo, pub, noopCashierLogger{})
	svc := NewReturnsService(uc)
	ctx := ctxWithTenantAndActor(1, 42)

	saleItems := salesRepo.items[saleID]
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "1"},
		},
	})
	require.Error(t, err)
	assert.Len(t, pub.events, 0)
}

func TestCreateReturn_DuplicateSaleItemIdOverReturnRejected(t *testing.T) {
	svc, salesRepo, _, _ := setupReturnService()

	saleID := createSaleInRepo(salesRepo, 1, 42, []data.SaleItemDto{
		{ProductID: 100, ProductName: "A", Quantity: decimal.NewFromInt(7), UnitPrice: decimal.NewFromInt(100), Total: decimal.NewFromInt(700)},
	})
	saleItems := salesRepo.items[saleID]

	ctx := ctxWithTenantAndActor(1, 42)
	// Two entries for the same sale_item_id summing to 10 > 7
	_, err := svc.CreateReturn(ctx, &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItems[0].ID, Quantity: "5"},
			{SaleItemId: saleItems[0].ID, Quantity: "5"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds remaining")
}

// --- Failing returns repo ---

type failingReturnsRepo struct{}

func (r *failingReturnsRepo) Create(_ context.Context, _ data.ReturnDto) (*ent.SaleReturn, error) {
	return nil, assert.AnError
}

func (r *failingReturnsRepo) GetReturnedQuantities(_ context.Context, _ int64) (map[int64]decimal.Decimal, error) {
	return make(map[int64]decimal.Decimal), nil
}

func (r *failingReturnsRepo) GetByUUID(_ context.Context, _ string) (*ent.SaleReturn, error) {
	return nil, assert.AnError
}
