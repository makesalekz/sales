package service

import (
	"context"
	"time"

	v1 "github.com/makesalekz/sales/api/sales/v1"
	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/internal/biz"
	"github.com/makesalekz/utils/v2/auth"
)

type CashierLogServiceImpl struct {
	v1.UnimplementedCashierLogServiceServer

	uc *biz.CashierLogUsecase
}

func NewCashierLogService(uc *biz.CashierLogUsecase) *CashierLogServiceImpl {
	return &CashierLogServiceImpl{uc: uc}
}

func (s *CashierLogServiceImpl) GetCashierLog(ctx context.Context, req *v1.GetCashierLogRequest) (*v1.GetCashierLogReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	filter := biz.CashierLogFilter{
		TenantID:  tenantID,
		CashierID: req.GetCashierId(),
		ShiftID:   req.GetShiftId(),
		Limit:     req.GetLimit(),
		FromID:    req.GetFromId(),
	}

	if req.GetDateFrom() != "" {
		t, err := time.Parse(time.RFC3339, req.GetDateFrom())
		if err != nil {
			return nil, v1.ErrorInvalidRequest("invalid date_from format, use RFC3339")
		}
		filter.DateFrom = &t
	}

	if req.GetDateTo() != "" {
		t, err := time.Parse(time.RFC3339, req.GetDateTo())
		if err != nil {
			return nil, v1.ErrorInvalidRequest("invalid date_to format, use RFC3339")
		}
		filter.DateTo = &t
	}

	entries, err := s.uc.GetCashierLog(ctx, filter)
	if err != nil {
		return nil, v1.ErrorInternal("%s", err.Error())
	}

	result := make([]*v1.CashierLogEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, replyCashierLogEntry(e))
	}

	return &v1.GetCashierLogReply{Entries: result}, nil
}

func replyCashierLogEntry(e *ent.CashierLog) *v1.CashierLogEntry {
	return &v1.CashierLogEntry{
		Id:          e.ID,
		TenantId:    e.TenantID,
		CashierId:   e.CashierID,
		ShiftId:     e.ShiftID,
		Action:      string(e.Action),
		EntityId:    e.EntityID,
		EntityType:  e.EntityType,
		Amount:      e.Amount.String(),
		Description: e.Description,
		CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
