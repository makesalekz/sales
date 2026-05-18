package service

import (
	"context"

	v1 "github.com/makesalekz/sales/api/sales/v1"
	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/internal/biz"
	"github.com/makesalekz/utils/v2/auth"
)

type ReturnsServiceImpl struct {
	v1.UnimplementedReturnsServiceServer

	uc *biz.ReturnsUsecase
}

func NewReturnsService(uc *biz.ReturnsUsecase) *ReturnsServiceImpl {
	return &ReturnsServiceImpl{uc: uc}
}

func (s *ReturnsServiceImpl) CreateReturn(ctx context.Context, req *v1.CreateReturnRequest) (*v1.CreateReturnReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	cashierID := auth.GetActorIdFromContext(ctx)

	if len(req.GetItems()) == 0 {
		return nil, v1.ErrorInvalidRequest("return must have at least one item")
	}

	items := make([]biz.CreateReturnItemInput, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, biz.CreateReturnItemInput{
			SaleItemID: item.GetSaleItemId(),
			Quantity:   parseDecimal(item.GetQuantity()),
		})
	}

	input := biz.CreateReturnInput{
		TenantID:  tenantID,
		CashierID: cashierID,
		SaleID:    req.GetSaleId(),
		Items:     items,
	}

	saleReturn, err := s.uc.CreateReturn(ctx, input)
	if err != nil {
		return nil, v1.ErrorInvalidRequest("%s", err.Error())
	}

	return &v1.CreateReturnReply{SaleReturn: replyReturn(saleReturn)}, nil
}

func replyReturn(sr *ent.SaleReturn) *v1.Return {
	resp := &v1.Return{
		Id:        sr.ID,
		TenantId:  sr.TenantID,
		SaleId:    sr.SaleID,
		CashierId: sr.CashierID,
		Total:     sr.Total.String(),
		CreatedAt: sr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if sr.Edges.Items != nil {
		for _, item := range sr.Edges.Items {
			resp.Items = append(resp.Items, replyReturnItem(item))
		}
	}

	return resp
}

func replyReturnItem(item *ent.SaleReturnItem) *v1.ReturnItem {
	return &v1.ReturnItem{
		Id:         item.ID,
		ReturnId:   item.SaleReturnID,
		SaleItemId: item.SaleItemID,
		ProductId:  item.ProductID,
		Quantity:   item.Quantity.String(),
		UnitPrice:  item.UnitPrice.String(),
		Total:      item.Total.String(),
	}
}
