package service

import (
	"context"
	"testing"
	"time"

	v1 "github.com/makesalekz/sales/api/sales/v1"
	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/internal/biz"
	"github.com/makesalekz/sales/internal/data"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// --- Sync Mock ReturnsRepo ---

type syncMockReturnsRepo struct {
	returns     map[int64]*ent.SaleReturn
	returnItems map[int64][]*ent.SaleReturnItem
	nextID      int64
	itemID      int64
}

func newSyncMockReturnsRepo() *syncMockReturnsRepo {
	return &syncMockReturnsRepo{
		returns:     make(map[int64]*ent.SaleReturn),
		returnItems: make(map[int64][]*ent.SaleReturnItem),
		nextID:      1,
		itemID:      1,
	}
}

func (m *syncMockReturnsRepo) Create(_ context.Context, dto data.ReturnDto) (*ent.SaleReturn, error) {
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

	var items []*ent.SaleReturnItem
	for _, item := range dto.Items {
		ri := &ent.SaleReturnItem{
			ID:           m.itemID,
			SaleReturnID: sr.ID,
			SaleItemID:   item.SaleItemID,
			ProductID:    item.ProductID,
			Quantity:     item.Quantity,
			UnitPrice:    item.UnitPrice,
			Total:        item.Total,
		}
		items = append(items, ri)
		m.itemID++
	}
	m.returnItems[sr.ID] = items
	sr.Edges.Items = items

	m.nextID++
	return sr, nil
}

func (m *syncMockReturnsRepo) GetReturnedQuantities(_ context.Context, saleID int64) (map[int64]decimal.Decimal, error) {
	result := make(map[int64]decimal.Decimal)
	for _, sr := range m.returns {
		if sr.SaleID != saleID {
			continue
		}
		for _, item := range m.returnItems[sr.ID] {
			result[item.SaleItemID] = result[item.SaleItemID].Add(item.Quantity)
		}
	}
	return result, nil
}

func (m *syncMockReturnsRepo) GetByUUID(_ context.Context, uuid string) (*ent.SaleReturn, error) {
	for _, sr := range m.returns {
		if sr.UUID != nil && *sr.UUID == uuid {
			sr.Edges.Items = m.returnItems[sr.ID]
			return sr, nil
		}
	}
	return nil, assert.AnError
}

// --- Sync Mock ReturnPublisher ---

type syncMockReturnPublisher struct {
	events []data.ReturnCompletedEvent
}

func (p *syncMockReturnPublisher) Publish(event data.ReturnCompletedEvent) {
	p.events = append(p.events, event)
}

// --- Sync-aware mock SalesRepo ---

type syncMockSalesRepo struct {
	*mockSalesRepo
}

func newSyncMockSalesRepo() *syncMockSalesRepo {
	return &syncMockSalesRepo{mockSalesRepo: newMockSalesRepo()}
}

func (m *syncMockSalesRepo) Create(ctx context.Context, dto data.SaleDto) (*ent.Sale, error) {
	s, err := m.mockSalesRepo.Create(ctx, dto)
	if err != nil {
		return nil, err
	}
	if dto.UUID != "" {
		s.UUID = &dto.UUID
	}
	return s, nil
}

// --- Test setup ---

func setupSyncService() (*SyncServiceImpl, *syncMockSalesRepo, *syncMockReturnsRepo) {
	salesRepo := newSyncMockSalesRepo()
	returnsRepo := newSyncMockReturnsRepo()
	pub := newMockPublisher()
	retPub := &syncMockReturnPublisher{}
	cl := noopCashierLogger{}

	salesUc := biz.NewSalesUsecase(log.DefaultLogger, salesRepo, pub, cl)
	returnsUc := biz.NewReturnsUsecase(log.DefaultLogger, returnsRepo, salesRepo, retPub, cl)
	syncUc := biz.NewSyncUsecase(log.DefaultLogger, salesUc, returnsUc, salesRepo, returnsRepo)
	svc := NewSyncService(syncUc)
	return svc, salesRepo, returnsRepo
}

func makeSaleData(t *testing.T) []byte {
	t.Helper()
	req := &v1.CreateSaleRequest{
		PaymentType: "CASH",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "Test", Quantity: "2", UnitPrice: "100"},
		},
	}
	b, err := proto.Marshal(req)
	require.NoError(t, err)
	return b
}

func makeReturnData(t *testing.T, saleID int64, saleItemID int64) []byte {
	t.Helper()
	req := &v1.CreateReturnRequest{
		SaleId: saleID,
		Items: []*v1.CreateReturnItemRequest{
			{SaleItemId: saleItemID, Quantity: "1"},
		},
	}
	b, err := proto.Marshal(req)
	require.NoError(t, err)
	return b
}

// --- Tests ---

func TestSync_NewSale_Synced(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{
				Uuid:      "uuid-sale-1",
				Type:      v1.SyncOperationType_SALE,
				Data:      makeSaleData(t),
				CreatedAt: "2026-05-17T10:00:00Z",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "uuid-sale-1", resp.Results[0].Uuid)
	assert.Equal(t, "synced", resp.Results[0].Status)
	assert.Empty(t, resp.Results[0].ErrorMessage)
}

func TestSync_DuplicateUUID_AlreadySynced(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	saleData := makeSaleData(t)

	// First sync
	resp1, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-dup-1", Type: v1.SyncOperationType_SALE, Data: saleData},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "synced", resp1.Results[0].Status)

	// Second sync with same UUID
	resp2, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-dup-1", Type: v1.SyncOperationType_SALE, Data: saleData},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "already_synced", resp2.Results[0].Status)
}

func TestSync_DuplicateUUIDWithinBatch(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	saleData := makeSaleData(t)

	resp, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-batch-dup", Type: v1.SyncOperationType_SALE, Data: saleData},
			{Uuid: "uuid-batch-dup", Type: v1.SyncOperationType_SALE, Data: saleData},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "synced", resp.Results[0].Status)
	assert.Equal(t, "already_synced", resp.Results[1].Status)
}

func TestSync_BatchPartialFailure(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	goodData := makeSaleData(t)
	badData := []byte("not-valid-proto")

	resp, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-good", Type: v1.SyncOperationType_SALE, Data: goodData},
			{Uuid: "uuid-bad", Type: v1.SyncOperationType_SALE, Data: badData},
			{Uuid: "uuid-good2", Type: v1.SyncOperationType_SALE, Data: goodData},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 3)

	assert.Equal(t, "synced", resp.Results[0].Status)
	assert.Equal(t, "error", resp.Results[1].Status)
	assert.Contains(t, resp.Results[1].ErrorMessage, "failed to deserialize")
	assert.Equal(t, "synced", resp.Results[2].Status)
}

func TestSync_InvalidProtoBytes_Error(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-garbage", Type: v1.SyncOperationType_SALE, Data: []byte{0xFF, 0xFE, 0xFD}},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "error", resp.Results[0].Status)
	assert.NotEmpty(t, resp.Results[0].ErrorMessage)
}

func TestSync_EmptyUUID_Error(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	resp, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "", Type: v1.SyncOperationType_SALE, Data: makeSaleData(t)},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "error", resp.Results[0].Status)
	assert.Contains(t, resp.Results[0].ErrorMessage, "uuid is required")
}

func TestSync_NoTenant_Error(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := context.Background()

	_, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-1", Type: v1.SyncOperationType_SALE, Data: makeSaleData(t)},
		},
	})
	require.Error(t, err)
}

func TestSync_EmptyOperations_Error(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	_, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{},
	})
	require.Error(t, err)
}

func TestSync_ReturnOperation(t *testing.T) {
	svc, salesRepo, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	// First create a sale so we can return against it
	saleData := makeSaleData(t)
	resp1, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-sale-for-return", Type: v1.SyncOperationType_SALE, Data: saleData},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "synced", resp1.Results[0].Status)

	// Find the created sale
	var saleID int64
	var saleItemID int64
	for _, s := range salesRepo.sales {
		saleID = s.ID
		if items := salesRepo.items[s.ID]; len(items) > 0 {
			saleItemID = items[0].ID
		}
		break
	}
	require.NotZero(t, saleID)
	require.NotZero(t, saleItemID)

	// Sync a return
	returnData := makeReturnData(t, saleID, saleItemID)
	resp2, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-return-1", Type: v1.SyncOperationType_RETURN, Data: returnData},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp2.Results, 1)
	assert.Equal(t, "synced", resp2.Results[0].Status)
}

func TestSync_ReturnDuplicate_AlreadySynced(t *testing.T) {
	svc, salesRepo, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Create a sale first
	saleData := makeSaleData(t)
	_, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-sale-ret-dup", Type: v1.SyncOperationType_SALE, Data: saleData},
		},
	})
	require.NoError(t, err)

	var saleID int64
	var saleItemID int64
	for _, s := range salesRepo.sales {
		saleID = s.ID
		if items := salesRepo.items[s.ID]; len(items) > 0 {
			saleItemID = items[0].ID
		}
		break
	}

	returnData := makeReturnData(t, saleID, saleItemID)

	// First return sync
	resp1, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-return-dup", Type: v1.SyncOperationType_RETURN, Data: returnData},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "synced", resp1.Results[0].Status)

	// Second return sync same UUID
	resp2, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-return-dup", Type: v1.SyncOperationType_RETURN, Data: returnData},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "already_synced", resp2.Results[0].Status)
}

func TestSync_MixedTypes(t *testing.T) {
	svc, salesRepo, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Create a sale first (outside sync) by syncing it
	saleData := makeSaleData(t)
	_, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-sale-mixed", Type: v1.SyncOperationType_SALE, Data: saleData},
		},
	})
	require.NoError(t, err)

	var saleID int64
	var saleItemID int64
	for _, s := range salesRepo.sales {
		saleID = s.ID
		if items := salesRepo.items[s.ID]; len(items) > 0 {
			saleItemID = items[0].ID
		}
		break
	}

	returnData := makeReturnData(t, saleID, saleItemID)

	// Mixed batch: new sale + return
	resp, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-new-sale", Type: v1.SyncOperationType_SALE, Data: saleData},
			{Uuid: "uuid-new-return", Type: v1.SyncOperationType_RETURN, Data: returnData},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "synced", resp.Results[0].Status)
	assert.Equal(t, "synced", resp.Results[1].Status)
}

func TestSync_InvalidSaleData_OtherOpsSucceed(t *testing.T) {
	svc, _, _ := setupSyncService()
	ctx := ctxWithTenantAndActor(1, 42)

	// Sale with invalid payment type
	badSale := &v1.CreateSaleRequest{
		PaymentType: "INVALID",
		Items: []*v1.CreateSaleItemRequest{
			{ProductId: 1, ProductName: "Test", Quantity: "1", UnitPrice: "100"},
		},
	}
	badData, err := proto.Marshal(badSale)
	require.NoError(t, err)

	goodData := makeSaleData(t)

	resp, err := svc.SyncOperations(ctx, &v1.SyncOperationsRequest{
		Operations: []*v1.SyncOperation{
			{Uuid: "uuid-ok-1", Type: v1.SyncOperationType_SALE, Data: goodData},
			{Uuid: "uuid-bad-payment", Type: v1.SyncOperationType_SALE, Data: badData},
			{Uuid: "uuid-ok-2", Type: v1.SyncOperationType_SALE, Data: goodData},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 3)

	assert.Equal(t, "synced", resp.Results[0].Status)
	assert.Equal(t, "error", resp.Results[1].Status)
	assert.Contains(t, resp.Results[1].ErrorMessage, "payment_type")
	assert.Equal(t, "synced", resp.Results[2].Status)
}
