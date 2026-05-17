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

type SalesUsecase struct {
	log       *log.Helper
	repo      data.SalesRepo
	publisher data.SaleCompletedPublisher
	cashierLog CashierLogger
}

func NewSalesUsecase(logger log.Logger, repo data.SalesRepo, publisher data.SaleCompletedPublisher, cashierLog CashierLogger) *SalesUsecase {
	return &SalesUsecase{
		log:        log.NewHelper(logger),
		repo:       repo,
		publisher:  publisher,
		cashierLog: cashierLog,
	}
}

type CreateSaleInput struct {
	UUID          string
	TenantID      int64
	CashierID     int64
	ShiftID       int64
	PaymentType   enum.PaymentType
	DiscountType  enum.DiscountType
	DiscountValue decimal.Decimal
	Items         []CreateSaleItemInput
}

type CreateSaleItemInput struct {
	ProductID   int64
	ProductName string
	Quantity    decimal.Decimal
	UnitPrice   decimal.Decimal
	Discount    decimal.Decimal
}

func (uc *SalesUsecase) CreateSale(ctx context.Context, input CreateSaleInput) (*ent.Sale, error) {
	if len(input.Items) == 0 {
		return nil, fmt.Errorf("sale must have at least one item")
	}

	if !input.PaymentType.IsValid() {
		return nil, fmt.Errorf("invalid payment type: %s", input.PaymentType)
	}

	// Validate whole-sale discount
	if input.DiscountType != enum.DiscountNone {
		if input.DiscountValue.IsNegative() {
			return nil, fmt.Errorf("discount_value must not be negative")
		}
		if input.DiscountType == enum.DiscountPercentage && input.DiscountValue.GreaterThan(decimal.NewFromInt(100)) {
			return nil, fmt.Errorf("percentage discount cannot exceed 100")
		}
	}

	// Calculate per-item totals (with per-item discounts)
	var subtotal decimal.Decimal
	var itemDiscountTotal decimal.Decimal
	items := make([]data.SaleItemDto, 0, len(input.Items))

	for _, item := range input.Items {
		// item.total = quantity * unit_price - discount
		itemTotal := item.Quantity.Mul(item.UnitPrice).Sub(item.Discount)
		subtotal = subtotal.Add(itemTotal)
		itemDiscountTotal = itemDiscountTotal.Add(item.Discount)

		items = append(items, data.SaleItemDto{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Discount:    item.Discount,
			Total:       itemTotal,
		})
	}

	// Apply whole-sale discount on top of the subtotal (after per-item discounts)
	saleTotal := subtotal
	wholeSaleDiscount := decimal.Zero
	switch input.DiscountType {
	case enum.DiscountPercentage:
		wholeSaleDiscount = subtotal.Mul(input.DiscountValue).Div(decimal.NewFromInt(100))
		saleTotal = subtotal.Sub(wholeSaleDiscount)
	case enum.DiscountFixed:
		if input.DiscountValue.GreaterThan(subtotal) {
			return nil, fmt.Errorf("fixed discount (%s) exceeds subtotal (%s)", input.DiscountValue, subtotal)
		}
		wholeSaleDiscount = input.DiscountValue
		saleTotal = subtotal.Sub(wholeSaleDiscount)
	}

	// discount_total = sum of per-item discounts + whole-sale discount
	discountTotal := itemDiscountTotal.Add(wholeSaleDiscount)

	dto := data.SaleDto{
		UUID:          input.UUID,
		TenantID:      input.TenantID,
		ShiftID:       input.ShiftID,
		CashierID:     input.CashierID,
		Total:         saleTotal,
		DiscountTotal: discountTotal,
		DiscountType:  input.DiscountType,
		DiscountValue: input.DiscountValue,
		PaymentType:   input.PaymentType,
		Status:        enum.Completed,
		Items:         items,
	}

	sale, err := uc.repo.Create(ctx, dto)
	if err != nil {
		return nil, err
	}

	// Log cashier action (best-effort)
	uc.cashierLog.Log(ctx, CashierLogEntry{
		TenantID:    input.TenantID,
		CashierID:   input.CashierID,
		ShiftID:     input.ShiftID,
		Action:      enum.ActionSale,
		EntityID:    sale.ID,
		EntityType:  "sale",
		Amount:      saleTotal,
		Description: fmt.Sprintf("Sale #%d completed", sale.ID),
	})

	// Publish event
	eventItems := make([]data.SaleCompletedEventItem, 0, len(items))
	for _, item := range items {
		eventItems = append(eventItems, data.SaleCompletedEventItem{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity.String(),
			UnitPrice:   item.UnitPrice.String(),
			Discount:    item.Discount.String(),
			Total:       item.Total.String(),
		})
	}

	uc.publisher.Publish(data.SaleCompletedEvent{
		TenantID:  input.TenantID,
		SaleID:    sale.ID,
		Total:     saleTotal.String(),
		Items:     eventItems,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	return sale, nil
}
