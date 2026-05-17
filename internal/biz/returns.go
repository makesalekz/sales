package biz

import (
	"context"
	"fmt"
	"time"

	"gitlab.calendaria.team/services/sales/ent"
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/internal/data"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
)

type ReturnsUsecase struct {
	log           *log.Helper
	returnsRepo   data.ReturnsRepo
	salesRepo     data.SalesRepo
	publisher     data.ReturnCompletedPublisher
	cashierLog    CashierLogger
}

func NewReturnsUsecase(
	logger log.Logger,
	returnsRepo data.ReturnsRepo,
	salesRepo data.SalesRepo,
	publisher data.ReturnCompletedPublisher,
	cashierLog CashierLogger,
) *ReturnsUsecase {
	return &ReturnsUsecase{
		log:         log.NewHelper(logger),
		returnsRepo: returnsRepo,
		salesRepo:   salesRepo,
		publisher:   publisher,
		cashierLog:  cashierLog,
	}
}

type CreateReturnItemInput struct {
	SaleItemID int64
	Quantity   decimal.Decimal
}

type CreateReturnInput struct {
	UUID      string
	TenantID  int64
	CashierID int64
	SaleID    int64
	Items     []CreateReturnItemInput
}

func (uc *ReturnsUsecase) CreateReturn(ctx context.Context, input CreateReturnInput) (*ent.SaleReturn, error) {
	if len(input.Items) == 0 {
		return nil, fmt.Errorf("return must have at least one item")
	}

	// Fetch sale with items
	sale, err := uc.salesRepo.GetByIDWithItems(ctx, input.SaleID, input.TenantID)
	if err != nil {
		return nil, fmt.Errorf("sale not found")
	}

	if sale.Status == enum.Returned {
		return nil, fmt.Errorf("sale is already fully returned")
	}

	// Build map of sale items by ID for lookup
	saleItemMap := make(map[int64]*ent.SaleItem)
	for _, item := range sale.Edges.Items {
		saleItemMap[item.ID] = item
	}

	// Get already returned quantities
	returnedQty, err := uc.returnsRepo.GetReturnedQuantities(ctx, input.SaleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get returned quantities: %w", err)
	}

	// Validate and compute (track consumed qty to handle duplicate sale_item_id in request)
	var returnTotal decimal.Decimal
	consumed := make(map[int64]decimal.Decimal)
	items := make([]data.ReturnItemDto, 0, len(input.Items))

	for _, reqItem := range input.Items {
		saleItem, ok := saleItemMap[reqItem.SaleItemID]
		if !ok {
			return nil, fmt.Errorf("sale_item_id %d does not belong to sale %d", reqItem.SaleItemID, input.SaleID)
		}

		if reqItem.Quantity.LessThanOrEqual(decimal.Zero) {
			return nil, fmt.Errorf("return quantity must be greater than zero")
		}

		alreadyReturned := returnedQty[reqItem.SaleItemID]
		remaining := saleItem.Quantity.Sub(alreadyReturned).Sub(consumed[reqItem.SaleItemID])

		if reqItem.Quantity.GreaterThan(remaining) {
			return nil, fmt.Errorf("return quantity %s exceeds remaining %s for sale_item_id %d",
				reqItem.Quantity.String(), remaining.String(), reqItem.SaleItemID)
		}

		consumed[reqItem.SaleItemID] = consumed[reqItem.SaleItemID].Add(reqItem.Quantity)

		itemTotal := saleItem.UnitPrice.Mul(reqItem.Quantity)
		returnTotal = returnTotal.Add(itemTotal)

		items = append(items, data.ReturnItemDto{
			SaleItemID: reqItem.SaleItemID,
			ProductID:  saleItem.ProductID,
			Quantity:   reqItem.Quantity,
			UnitPrice:  saleItem.UnitPrice,
			Total:      itemTotal,
		})
	}

	// Create return
	dto := data.ReturnDto{
		UUID:      input.UUID,
		TenantID:  input.TenantID,
		SaleID:    input.SaleID,
		CashierID: input.CashierID,
		Total:     returnTotal,
		Items:     items,
	}

	saleReturn, err := uc.returnsRepo.Create(ctx, dto)
	if err != nil {
		return nil, err
	}

	// Log cashier action (best-effort)
	uc.cashierLog.Log(ctx, CashierLogEntry{
		TenantID:    input.TenantID,
		CashierID:   input.CashierID,
		ShiftID:     sale.ShiftID,
		Action:      enum.ActionReturn,
		EntityID:    saleReturn.ID,
		EntityType:  "return",
		Amount:      returnTotal,
		Description: fmt.Sprintf("Return #%d for sale #%d", saleReturn.ID, input.SaleID),
	})

	// Check if sale is now fully returned (use consumed map which sums all request items per sale_item_id)
	fullyReturned := true
	for _, saleItem := range sale.Edges.Items {
		totalReturned := returnedQty[saleItem.ID].Add(consumed[saleItem.ID])
		if totalReturned.LessThan(saleItem.Quantity) {
			fullyReturned = false
			break
		}
	}

	if fullyReturned {
		if err := uc.salesRepo.UpdateStatus(ctx, input.SaleID, enum.Returned); err != nil {
			uc.log.Errorf("failed to update sale status to RETURNED: %v", err)
		}
	}

	// Publish event with negative total
	eventItems := make([]data.ReturnCompletedEventItem, 0, len(items))
	for _, item := range items {
		eventItems = append(eventItems, data.ReturnCompletedEventItem{
			ProductID:  item.ProductID,
			SaleItemID: item.SaleItemID,
			Quantity:   item.Quantity.String(),
			UnitPrice:  item.UnitPrice.String(),
			Total:      item.Total.Neg().String(),
		})
	}

	uc.publisher.Publish(data.ReturnCompletedEvent{
		TenantID:  input.TenantID,
		SaleID:    input.SaleID,
		ReturnID:  saleReturn.ID,
		Total:     returnTotal.Neg().String(),
		Items:     eventItems,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	return saleReturn, nil
}
