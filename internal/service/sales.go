package service

import (
	"context"

	v1 "gitlab.calendaria.team/services/sales/api/sales/v1"
	"gitlab.calendaria.team/services/sales/ent"
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/internal/biz"
	"gitlab.calendaria.team/services/utils/v2/auth"

	"github.com/shopspring/decimal"
)

type SalesService struct {
	v1.UnimplementedSalesServiceServer

	uc *biz.SalesUsecase
}

func NewSalesService(uc *biz.SalesUsecase) *SalesService {
	return &SalesService{uc: uc}
}

func (s *SalesService) CreateSale(ctx context.Context, req *v1.CreateSaleRequest) (*v1.CreateSaleReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	cashierID := auth.GetActorIdFromContext(ctx)

	paymentType := enum.PaymentType(req.GetPaymentType())
	if !paymentType.IsValid() {
		return nil, v1.ErrorInvalidRequest("invalid payment_type, must be CASH, CARD, or MIXED")
	}

	if len(req.GetItems()) == 0 {
		return nil, v1.ErrorInvalidRequest("sale must have at least one item")
	}

	items := make([]biz.CreateSaleItemInput, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, biz.CreateSaleItemInput{
			ProductID:   item.GetProductId(),
			ProductName: item.GetProductName(),
			Quantity:    parseDecimal(item.GetQuantity()),
			UnitPrice:   parseDecimal(item.GetUnitPrice()),
			Discount:    parseDecimal(item.GetDiscount()),
		})
	}

	// Parse whole-sale discount: empty string treated as NONE for backward compat
	discountType := enum.DiscountNone
	if dt := req.GetDiscountType(); dt != "" {
		discountType = enum.DiscountType(dt)
		if !discountType.IsValid() {
			return nil, v1.ErrorInvalidRequest("invalid discount_type, must be NONE, PERCENTAGE, or FIXED")
		}
	}

	input := biz.CreateSaleInput{
		TenantID:      tenantID,
		CashierID:     cashierID,
		ShiftID:       req.GetShiftId(),
		PaymentType:   paymentType,
		DiscountType:  discountType,
		DiscountValue: parseDecimal(req.GetDiscountValue()),
		Items:         items,
	}

	sale, err := s.uc.CreateSale(ctx, input)
	if err != nil {
		return nil, v1.ErrorInvalidRequest("%s", err.Error())
	}

	return &v1.CreateSaleReply{Sale: replySale(sale)}, nil
}

func replySale(s *ent.Sale) *v1.Sale {
	resp := &v1.Sale{
		Id:            s.ID,
		TenantId:      s.TenantID,
		ShiftId:       s.ShiftID,
		CashierId:     s.CashierID,
		Total:         s.Total.String(),
		DiscountTotal: s.DiscountTotal.String(),
		DiscountType:  string(s.DiscountType),
		DiscountValue: s.DiscountValue.String(),
		PaymentType:   string(s.PaymentType),
		Status:        string(s.Status),
		CreatedAt:     s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if s.Edges.Items != nil {
		for _, item := range s.Edges.Items {
			resp.Items = append(resp.Items, replySaleItem(item))
		}
	}

	return resp
}

func replySaleItem(item *ent.SaleItem) *v1.SaleItem {
	return &v1.SaleItem{
		Id:          item.ID,
		SaleId:      item.SaleID,
		ProductId:   item.ProductID,
		ProductName: item.ProductName,
		Quantity:    item.Quantity.String(),
		UnitPrice:   item.UnitPrice.String(),
		Discount:    item.Discount.String(),
		Total:       item.Total.String(),
	}
}

func parseDecimal(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}
