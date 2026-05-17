package service

import (
	"context"

	v1 "gitlab.calendaria.team/services/sales/api/sales/v1"
	"gitlab.calendaria.team/services/sales/ent"
	"gitlab.calendaria.team/services/sales/internal/biz"
	"gitlab.calendaria.team/services/utils/v2/auth"

	"github.com/shopspring/decimal"
)

type ShiftServiceImpl struct {
	v1.UnimplementedShiftServiceServer

	uc *biz.ShiftsUsecase
}

func NewShiftService(uc *biz.ShiftsUsecase) *ShiftServiceImpl {
	return &ShiftServiceImpl{uc: uc}
}

func (s *ShiftServiceImpl) OpenShift(ctx context.Context, req *v1.OpenShiftRequest) (*v1.OpenShiftReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	cashierID := auth.GetActorIdFromContext(ctx)
	if cashierID == 0 {
		return nil, v1.ErrorInvalidRequest("empty actor id")
	}

	openingAmount, _ := decimal.NewFromString(req.GetOpeningAmount())

	input := biz.OpenShiftInput{
		TenantID:      tenantID,
		CashierID:     cashierID,
		OpeningAmount: openingAmount,
	}
	if req.StoreId != nil {
		sid := *req.StoreId
		input.StoreID = &sid
	}

	shift, err := s.uc.OpenShift(ctx, input)
	if err != nil {
		return nil, v1.ErrorAlreadyExists("%s", err.Error())
	}

	return &v1.OpenShiftReply{Shift: replyShift(shift)}, nil
}

func (s *ShiftServiceImpl) CloseShift(ctx context.Context, req *v1.CloseShiftRequest) (*v1.CloseShiftReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	cashierID := auth.GetActorIdFromContext(ctx)
	if cashierID == 0 {
		return nil, v1.ErrorInvalidRequest("empty actor id")
	}

	result, err := s.uc.CloseShift(ctx, req.GetShiftId(), tenantID, cashierID)
	if err != nil {
		return nil, v1.ErrorInvalidRequest("%s", err.Error())
	}

	zReport := &v1.ZReport{
		ShiftId:       result.Shift.ID,
		CashierId:     result.Shift.CashierID,
		OpeningAmount: result.Shift.OpeningAmount.String(),
		ClosingAmount: result.Shift.ClosingAmount.String(),
		TotalSales:    result.Shift.TotalSales.String(),
		TotalReturns:  result.Shift.TotalReturns.String(),
		ExpectedCash:  result.Shift.ClosingAmount.String(),
		SalesCount:    result.Summary.SalesCount,
		ReturnsCount:  result.Summary.ReturnsCount,
		OpenedAt:      result.Shift.OpenedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if result.Shift.ClosedAt != nil {
		zReport.ClosedAt = result.Shift.ClosedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return &v1.CloseShiftReply{
		Shift:   replyShift(result.Shift),
		ZReport: zReport,
	}, nil
}

func (s *ShiftServiceImpl) GetShift(ctx context.Context, req *v1.GetShiftRequest) (*v1.GetShiftReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	shift, err := s.uc.GetShift(ctx, req.GetShiftId(), tenantID)
	if err != nil {
		return nil, v1.ErrorNotFound("%s", err.Error())
	}

	return &v1.GetShiftReply{Shift: replyShift(shift)}, nil
}

func (s *ShiftServiceImpl) ListShifts(ctx context.Context, req *v1.ListShiftsRequest) (*v1.ListShiftsReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	var storeID *int64
	if req.StoreId != nil {
		sid := *req.StoreId
		storeID = &sid
	}

	shifts, err := s.uc.ListShifts(ctx, tenantID, storeID, req.GetLimit(), req.GetFromId())
	if err != nil {
		return nil, v1.ErrorInvalidRequest("%s", err.Error())
	}

	result := make([]*v1.Shift, 0, len(shifts))
	for _, sh := range shifts {
		result = append(result, replyShift(sh))
	}

	return &v1.ListShiftsReply{Shifts: result}, nil
}

func replyShift(s *ent.Shift) *v1.Shift {
	resp := &v1.Shift{
		Id:            s.ID,
		TenantId:      s.TenantID,
		CashierId:     s.CashierID,
		OpeningAmount: s.OpeningAmount.String(),
		ClosingAmount: s.ClosingAmount.String(),
		TotalSales:    s.TotalSales.String(),
		TotalReturns:  s.TotalReturns.String(),
		Status:        string(s.Status),
		OpenedAt:      s.OpenedAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedAt:     s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if s.ClosedAt != nil {
		resp.ClosedAt = s.ClosedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if s.StoreID != nil {
		resp.StoreId = s.StoreID
	}
	return resp
}
