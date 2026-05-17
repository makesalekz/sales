package biz

import (
	"context"
	"fmt"

	v1 "gitlab.calendaria.team/services/sales/api/sales/v1"
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/internal/data"
	"gitlab.calendaria.team/services/utils/v2/auth"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"
)

type SyncUsecase struct {
	log         *log.Helper
	salesUc     *SalesUsecase
	returnsUc   *ReturnsUsecase
	salesRepo   data.SalesRepo
	returnsRepo data.ReturnsRepo
}

func NewSyncUsecase(
	logger log.Logger,
	salesUc *SalesUsecase,
	returnsUc *ReturnsUsecase,
	salesRepo data.SalesRepo,
	returnsRepo data.ReturnsRepo,
) *SyncUsecase {
	return &SyncUsecase{
		log:         log.NewHelper(logger),
		salesUc:     salesUc,
		returnsUc:   returnsUc,
		salesRepo:   salesRepo,
		returnsRepo: returnsRepo,
	}
}

type SyncOperationInput struct {
	UUID      string
	Type      v1.SyncOperationType
	Data      []byte
	CreatedAt string
}

type SyncOperationOutput struct {
	UUID         string
	Status       string // "synced", "already_synced", "error"
	ErrorMessage string
}

func (uc *SyncUsecase) SyncOperations(ctx context.Context, ops []SyncOperationInput) []SyncOperationOutput {
	results := make([]SyncOperationOutput, 0, len(ops))
	synced := make(map[string]bool) // true = successfully synced in this batch

	for _, op := range ops {
		if op.UUID == "" {
			results = append(results, SyncOperationOutput{
				UUID:         op.UUID,
				Status:       "error",
				ErrorMessage: "uuid is required",
			})
			continue
		}

		// Dedup within same batch: only skip if already successfully synced
		if synced[op.UUID] {
			results = append(results, SyncOperationOutput{
				UUID:   op.UUID,
				Status: "already_synced",
			})
			continue
		}

		result := uc.processOperation(ctx, op)
		if result.Status == "synced" {
			synced[op.UUID] = true
		}
		results = append(results, result)
	}

	return results
}

func (uc *SyncUsecase) processOperation(ctx context.Context, op SyncOperationInput) SyncOperationOutput {
	switch op.Type {
	case v1.SyncOperationType_SALE:
		return uc.processSale(ctx, op)
	case v1.SyncOperationType_RETURN:
		return uc.processReturn(ctx, op)
	default:
		return SyncOperationOutput{
			UUID:         op.UUID,
			Status:       "error",
			ErrorMessage: fmt.Sprintf("unknown operation type: %v", op.Type),
		}
	}
}

func (uc *SyncUsecase) processSale(ctx context.Context, op SyncOperationInput) SyncOperationOutput {
	// Check idempotency
	existing, err := uc.salesRepo.GetByUUID(ctx, op.UUID)
	if err == nil && existing != nil {
		return SyncOperationOutput{UUID: op.UUID, Status: "already_synced"}
	}

	// Deserialize
	var req v1.CreateSaleRequest
	if err := proto.Unmarshal(op.Data, &req); err != nil {
		return SyncOperationOutput{
			UUID:         op.UUID,
			Status:       "error",
			ErrorMessage: fmt.Sprintf("failed to deserialize sale data: %v", err),
		}
	}

	tenantID := auth.GetTenantIdFromContext(ctx)
	cashierID := auth.GetActorIdFromContext(ctx)

	paymentType := enum.PaymentType(req.GetPaymentType())
	if !paymentType.IsValid() {
		return SyncOperationOutput{
			UUID:         op.UUID,
			Status:       "error",
			ErrorMessage: "invalid payment_type",
		}
	}

	items := make([]CreateSaleItemInput, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, CreateSaleItemInput{
			ProductID:   item.GetProductId(),
			ProductName: item.GetProductName(),
			Quantity:    parseDecimal(item.GetQuantity()),
			UnitPrice:   parseDecimal(item.GetUnitPrice()),
			Discount:    parseDecimal(item.GetDiscount()),
		})
	}

	discountType := enum.DiscountNone
	if dt := req.GetDiscountType(); dt != "" {
		discountType = enum.DiscountType(dt)
	}

	input := CreateSaleInput{
		UUID:          op.UUID,
		TenantID:      tenantID,
		CashierID:     cashierID,
		ShiftID:       req.GetShiftId(),
		PaymentType:   paymentType,
		DiscountType:  discountType,
		DiscountValue: parseDecimal(req.GetDiscountValue()),
		Items:         items,
	}

	_, err = uc.salesUc.CreateSale(ctx, input)
	if err != nil {
		// Check if it was a unique constraint violation (concurrent sync)
		if existAfter, errGet := uc.salesRepo.GetByUUID(ctx, op.UUID); errGet == nil && existAfter != nil {
			return SyncOperationOutput{UUID: op.UUID, Status: "already_synced"}
		}
		return SyncOperationOutput{
			UUID:         op.UUID,
			Status:       "error",
			ErrorMessage: err.Error(),
		}
	}

	return SyncOperationOutput{UUID: op.UUID, Status: "synced"}
}

func (uc *SyncUsecase) processReturn(ctx context.Context, op SyncOperationInput) SyncOperationOutput {
	// Check idempotency
	existing, err := uc.returnsRepo.GetByUUID(ctx, op.UUID)
	if err == nil && existing != nil {
		return SyncOperationOutput{UUID: op.UUID, Status: "already_synced"}
	}

	// Deserialize
	var req v1.CreateReturnRequest
	if err := proto.Unmarshal(op.Data, &req); err != nil {
		return SyncOperationOutput{
			UUID:         op.UUID,
			Status:       "error",
			ErrorMessage: fmt.Sprintf("failed to deserialize return data: %v", err),
		}
	}

	tenantID := auth.GetTenantIdFromContext(ctx)
	cashierID := auth.GetActorIdFromContext(ctx)

	items := make([]CreateReturnItemInput, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, CreateReturnItemInput{
			SaleItemID: item.GetSaleItemId(),
			Quantity:   parseDecimal(item.GetQuantity()),
		})
	}

	input := CreateReturnInput{
		UUID:      op.UUID,
		TenantID:  tenantID,
		CashierID: cashierID,
		SaleID:    req.GetSaleId(),
		Items:     items,
	}

	_, err = uc.returnsUc.CreateReturn(ctx, input)
	if err != nil {
		// Check if it was a unique constraint violation (concurrent sync)
		if existAfter, errGet := uc.returnsRepo.GetByUUID(ctx, op.UUID); errGet == nil && existAfter != nil {
			return SyncOperationOutput{UUID: op.UUID, Status: "already_synced"}
		}
		return SyncOperationOutput{
			UUID:         op.UUID,
			Status:       "error",
			ErrorMessage: err.Error(),
		}
	}

	return SyncOperationOutput{UUID: op.UUID, Status: "synced"}
}

func parseDecimal(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}
