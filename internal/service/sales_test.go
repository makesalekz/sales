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
	"gitlab.calendaria.team/services/utils/v2/auth"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock SalesRepo ---

type mockSalesRepo struct {
	sales  map[int64]*ent.Sale
	items  map[int64][]*ent.SaleItem
	nextID int64
	itemID int64
}

func newMockSalesRepo() *mockSalesRepo {
	return &mockSalesRepo{
		sales:  make(map[int64]*ent.Sale),
		items:  make(map[int64][]*ent.SaleItem),
		nextID: 1,
		itemID: 1,
	}
}

func (m *mockSalesRepo) Create(_ context.Context, dto data.SaleDto) (*ent.Sale, error) {
	var uuid *string
	if dto.UUID != "" {
		uuid = &dto.UUID
	}
	s := &ent.Sale{
		ID:            m.nextID,
		UUID:          uuid,
		TenantID:      dto.TenantID,
		ShiftID:       dto.ShiftID,
		CashierID:     dto.CashierID,
		Total:         dto.Total,
		DiscountTotal: dto.DiscountTotal,
		DiscountType:  dto.DiscountType,
		DiscountValue: dto.DiscountValue,
		PaymentType:   dto.PaymentType,
		Status:        dto.Status,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	m.sales[m.nextID] = s

	var saleItems []*ent.SaleItem
	for _, item := range dto.Items {
		si := &ent.SaleItem{
			ID:          m.itemID,
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
		m.itemID++
	}
	m.items[s.ID] = saleItems
	s.Edges.Items = saleItems

	m.nextID++
	return s, nil
}

func (m *mockSalesRepo) GetByIDWithItems(_ context.Context, id, tenantID int64) (*ent.Sale, error) {
	s, ok := m.sales[id]
	if !ok || s.TenantID != tenantID {
		return nil, assert.AnError
	}
	s.Edges.Items = m.items[id]
	return s, nil
}

func (m *mockSalesRepo) UpdateStatus(_ context.Context, id int64, status enum.SaleStatus) error {
	s, ok := m.sales[id]
	if !ok {
		return assert.AnError
	}
	s.Status = status
	return nil
}

func (m *mockSalesRepo) GetByUUID(_ context.Context, uuid string) (*ent.Sale, error) {
	for _, s := range m.sales {
		if s.UUID != nil && *s.UUID == uuid {
			s.Edges.Items = m.items[s.ID]
			return s, nil
		}
	}
	return nil, assert.AnError
}

// --- Mock Publisher ---

type mockPublisher struct {
	events []data.SaleCompletedEvent
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{}
}

func (p *mockPublisher) Publish(event data.SaleCompletedEvent) {
	p.events = append(p.events, event)
}

// --- Noop CashierLogger ---

type noopCashierLogger struct{}

func (noopCashierLogger) Log(_ context.Context, _ biz.CashierLogEntry) {}

// --- Test setup ---

func setupService() (*SalesService, *mockSalesRepo, *mockPublisher) {
	repo := newMockSalesRepo()
	pub := newMockPublisher()
	uc := biz.NewSalesUsecase(log.DefaultLogger, repo, pub, noopCashierLogger{})
	svc := NewSalesService(uc)
	return svc, repo, pub
}

func ctxWithTenantAndActor(tenantID, actorID int64) context.Context {
	ctx := auth.NewTenantContext(context.Background(), tenantID)
	return auth.NewActorContext(ctx, actorID)
}

// --- Tests ---

func TestCreateSale_HappyPath(t *testing.T) {
	svc, _, pub := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{
				ProductId:   100,
				ProductName: "Молоко",
				Quantity:    "2",
				UnitPrice:   "150",
				Discount:    "10",
			},
			{
				ProductId:   200,
				ProductName: "Хлеб",
				Quantity:    "1",
				UnitPrice:   "80",
				Discount:    "0",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Sale)

	assert.Equal(t, int64(1), resp.Sale.TenantId)
	assert.Equal(t, int64(42), resp.Sale.CashierId)
	assert.Equal(t, "CASH", resp.Sale.PaymentType)
	assert.Equal(t, "COMPLETED", resp.Sale.Status)
	// item1: 2*150 - 10 = 290
	// item2: 1*80 - 0 = 80
	// total: 370
	assert.Equal(t, "370", resp.Sale.Total)
	assert.Equal(t, "10", resp.Sale.DiscountTotal)
	assert.Len(t, resp.Sale.Items, 2)
	assert.NotEmpty(t, resp.Sale.CreatedAt)

	// Verify NATS event published
	require.Len(t, pub.events, 1)
	event := pub.events[0]
	assert.Equal(t, int64(1), event.TenantID)
	assert.Equal(t, resp.Sale.Id, event.SaleID)
	assert.Equal(t, "370", event.Total)
	assert.Len(t, event.Items, 2)
	assert.NotEmpty(t, event.Timestamp)
}

func TestCreateSale_NoTenant(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "Test", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateSale_EmptyItems(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items:       []*v1.CreateSaleItemRequest{},
	})
	require.Error(t, err)
}

func TestCreateSale_InvalidPaymentType(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "INVALID",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "Test", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateSale_ComputesItemTotals(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CARD",
		Items: []*v1.CreateSaleItemRequest{
			{
				ProductId:   1,
				ProductName: "Товар",
				Quantity:    "3",
				UnitPrice:   "100.50",
				Discount:    "5.50",
			},
		},
	})
	require.NoError(t, err)

	// 3 * 100.50 - 5.50 = 301.50 - 5.50 = 296
	assert.Equal(t, "296", resp.Sale.Items[0].Total)
	assert.Equal(t, "296", resp.Sale.Total)
	assert.Equal(t, "5.5", resp.Sale.DiscountTotal)
}

func TestCreateSale_AggregatesTotals(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "MIXED",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "2", UnitPrice: "100", Discount: "10"},
			{ProductId: 2, ProductName: "B", Quantity: "1", UnitPrice: "50", Discount: "5"},
			{ProductId: 3, ProductName: "C", Quantity: "5", UnitPrice: "20", Discount: "0"},
		},
	})
	require.NoError(t, err)

	// A: 2*100 - 10 = 190
	// B: 1*50 - 5 = 45
	// C: 5*20 - 0 = 100
	// total: 335
	// discount_total: 10 + 5 + 0 = 15
	assert.Equal(t, "335", resp.Sale.Total)
	assert.Equal(t, "15", resp.Sale.DiscountTotal)
}

func TestCreateSale_TenantIsolation(t *testing.T) {
	svc, repo, _ := setupService()
	ctx1 := ctxWithTenantAndActor(1, 42)
	ctx2 := ctxWithTenantAndActor(2, 43)

	resp1, err := svc.CreateSale(ctx1, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items:       []*v1.CreateSaleItemRequest{{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp1.Sale.TenantId)

	resp2, err := svc.CreateSale(ctx2, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items:       []*v1.CreateSaleItemRequest{{ProductId: 1, ProductName: "B", Quantity: "1", UnitPrice: "200"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp2.Sale.TenantId)

	// Different sales for different tenants
	assert.NotEqual(t, resp1.Sale.Id, resp2.Sale.Id)
	assert.Equal(t, 2, len(repo.sales))
}

func TestCreateSale_CashierIdFromContext(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 99)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items:       []*v1.CreateSaleItemRequest{{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(99), resp.Sale.CashierId)
}

func TestCreateSale_PublishesEventWithCorrectFields(t *testing.T) {
	svc, _, pub := setupService()
	ctx := ctxWithTenantAndActor(5, 10)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CARD",
		ShiftId:     7,
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 42, ProductName: "Чай", Quantity: "3", UnitPrice: "50", Discount: "5"},
		},
	})
	require.NoError(t, err)

	require.Len(t, pub.events, 1)
	event := pub.events[0]
	assert.Equal(t, int64(5), event.TenantID)
	assert.Equal(t, resp.Sale.Id, event.SaleID)
	assert.Equal(t, "145", event.Total) // 3*50 - 5 = 145
	require.Len(t, event.Items, 1)
	assert.Equal(t, int64(42), event.Items[0].ProductID)
	assert.Equal(t, "Чай", event.Items[0].ProductName)
	assert.Equal(t, "3", event.Items[0].Quantity)
	assert.Equal(t, "50", event.Items[0].UnitPrice)
	assert.Equal(t, "5", event.Items[0].Discount)
	assert.Equal(t, "145", event.Items[0].Total)
}

func TestCreateSale_DoesNotPublishOnRepoError(t *testing.T) {
	pub := newMockPublisher()
	// Use a failing repo
	repo := &failingRepo{}
	uc := biz.NewSalesUsecase(log.DefaultLogger, repo, pub, noopCashierLogger{})
	svc := NewSalesService(uc)
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items:       []*v1.CreateSaleItemRequest{{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"}},
	})
	require.Error(t, err)
	assert.Len(t, pub.events, 0)
}

func TestCreateSale_ShiftId(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		ShiftId:     123,
		Items:       []*v1.CreateSaleItemRequest{{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(123), resp.Sale.ShiftId)
}

func TestCreateSale_ZeroDiscount(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "100", resp.Sale.Total)
	assert.Equal(t, "0", resp.Sale.DiscountTotal)
	assert.Equal(t, "100", resp.Sale.Items[0].Total)
}

func TestCreateSale_DecimalPrecision(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "0.5", UnitPrice: "99.99", Discount: "0.01"},
		},
	})
	require.NoError(t, err)
	// 0.5 * 99.99 - 0.01 = 49.995 - 0.01 = 49.985
	assert.Equal(t, "49.985", resp.Sale.Total)
}

// --- Failing repo for error test ---

type failingRepo struct{}

func (r *failingRepo) Create(_ context.Context, _ data.SaleDto) (*ent.Sale, error) {
	return nil, assert.AnError
}

func (r *failingRepo) GetByIDWithItems(_ context.Context, _, _ int64) (*ent.Sale, error) {
	return nil, assert.AnError
}

func (r *failingRepo) UpdateStatus(_ context.Context, _ int64, _ enum.SaleStatus) error {
	return assert.AnError
}

func (r *failingRepo) GetByUUID(_ context.Context, _ string) (*ent.Sale, error) {
	return nil, assert.AnError
}

// --- Verify item-level detail in response ---

func TestCreateSale_ItemDetails(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 10, ProductName: "Кефир", Quantity: "2", UnitPrice: "80", Discount: "5"},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Sale.Items, 1)

	item := resp.Sale.Items[0]
	assert.Equal(t, int64(10), item.ProductId)
	assert.Equal(t, "Кефир", item.ProductName)
	assert.Equal(t, "2", item.Quantity)
	assert.Equal(t, "80", item.UnitPrice)
	assert.Equal(t, "5", item.Discount)
	assert.Equal(t, "155", item.Total) // 2*80 - 5 = 155
}

// Verify discount_total and total are correct for sales service
func TestCreateSale_MultipleItemsDiscountTotal(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100", Discount: "10"},
			{ProductId: 2, ProductName: "B", Quantity: "2", UnitPrice: "50", Discount: "20"},
		},
	})
	require.NoError(t, err)

	// A: 1*100 - 10 = 90
	// B: 2*50 - 20 = 80
	// total = 170
	// discount_total = 10 + 20 = 30
	d170 := decimal.NewFromInt(170)
	d30 := decimal.NewFromInt(30)
	saleTotal, _ := decimal.NewFromString(resp.Sale.Total)
	saleDiscount, _ := decimal.NewFromString(resp.Sale.DiscountTotal)
	assert.True(t, d170.Equal(saleTotal))
	assert.True(t, d30.Equal(saleDiscount))
}

// --- Whole-sale discount tests ---

func TestCreateSale_PercentageDiscount(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "PERCENTAGE",
		DiscountValue: "10",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "2", UnitPrice: "100"},
		},
	})
	require.NoError(t, err)

	// subtotal = 2*100 = 200
	// whole-sale discount = 200 * 10/100 = 20
	// total = 200 - 20 = 180
	// discount_total = 0 (per-item) + 20 (whole-sale) = 20
	assert.Equal(t, "180", resp.Sale.Total)
	assert.Equal(t, "20", resp.Sale.DiscountTotal)
	assert.Equal(t, "PERCENTAGE", resp.Sale.DiscountType)
	assert.Equal(t, "10", resp.Sale.DiscountValue)
}

func TestCreateSale_FixedDiscount(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "FIXED",
		DiscountValue: "50",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "2", UnitPrice: "100"},
		},
	})
	require.NoError(t, err)

	// subtotal = 200, fixed discount = 50, total = 150
	assert.Equal(t, "150", resp.Sale.Total)
	assert.Equal(t, "50", resp.Sale.DiscountTotal)
	assert.Equal(t, "FIXED", resp.Sale.DiscountType)
	assert.Equal(t, "50", resp.Sale.DiscountValue)
}

func TestCreateSale_PercentageWithPerItemDiscount(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "PERCENTAGE",
		DiscountValue: "10",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100", Discount: "10"},
			{ProductId: 2, ProductName: "B", Quantity: "1", UnitPrice: "100", Discount: "0"},
		},
	})
	require.NoError(t, err)

	// item A: 1*100 - 10 = 90
	// item B: 1*100 - 0 = 100
	// subtotal (after per-item discounts) = 190
	// whole-sale discount = 190 * 10/100 = 19
	// total = 190 - 19 = 171
	// discount_total = 10 (per-item) + 19 (whole-sale) = 29
	assert.Equal(t, "171", resp.Sale.Total)
	assert.Equal(t, "29", resp.Sale.DiscountTotal)
}

func TestCreateSale_NoDiscountType_BackwardCompat(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Old client: no discount_type or discount_value fields
	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "200"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "200", resp.Sale.Total)
	assert.Equal(t, "0", resp.Sale.DiscountTotal)
	assert.Equal(t, "NONE", resp.Sale.DiscountType)
	assert.Equal(t, "0", resp.Sale.DiscountValue)
}

func TestCreateSale_ExplicitNoneDiscount(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "NONE",
		DiscountValue: "0",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "100", resp.Sale.Total)
	assert.Equal(t, "0", resp.Sale.DiscountTotal)
}

func TestCreateSale_InvalidDiscountType(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "INVALID",
		DiscountValue: "10",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateSale_NegativeDiscountValue(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "FIXED",
		DiscountValue: "-10",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateSale_PercentageOver100(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "PERCENTAGE",
		DiscountValue: "150",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateSale_FixedExceedsSubtotal(t *testing.T) {
	svc, _, _ := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CASH",
		DiscountType:  "FIXED",
		DiscountValue: "500",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "100"},
		},
	})
	require.Error(t, err)
}

func TestCreateSale_PercentageDiscount_EventTotal(t *testing.T) {
	svc, _, pub := setupService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.CreateSale(ctx, &v1.CreateSaleRequest{
		PaymentType:   "CARD",
		DiscountType:  "PERCENTAGE",
		DiscountValue: "20",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "A", Quantity: "1", UnitPrice: "500"},
		},
	})
	require.NoError(t, err)

	// subtotal = 500, 20% = 100, total = 400
	assert.Equal(t, "400", resp.Sale.Total)

	require.Len(t, pub.events, 1)
	assert.Equal(t, "400", pub.events[0].Total)
}
