package service

import (
	"context"

	v1 "gitlab.calendaria.team/services/sales/api/sales/v1"
	"gitlab.calendaria.team/services/sales/internal/biz"
	"gitlab.calendaria.team/services/utils/v2/auth"
)

type SyncServiceImpl struct {
	v1.UnimplementedSyncServiceServer

	uc *biz.SyncUsecase
}

func NewSyncService(uc *biz.SyncUsecase) *SyncServiceImpl {
	return &SyncServiceImpl{uc: uc}
}

func (s *SyncServiceImpl) SyncOperations(ctx context.Context, req *v1.SyncOperationsRequest) (*v1.SyncOperationsReply, error) {
	tenantID := auth.GetTenantIdFromContext(ctx)
	if tenantID == 0 {
		return nil, v1.ErrorInvalidRequest("empty tenant id")
	}

	if len(req.GetOperations()) == 0 {
		return nil, v1.ErrorInvalidRequest("operations list is empty")
	}

	ops := make([]biz.SyncOperationInput, 0, len(req.GetOperations()))
	for _, op := range req.GetOperations() {
		ops = append(ops, biz.SyncOperationInput{
			UUID:      op.GetUuid(),
			Type:      op.GetType(),
			Data:      op.GetData(),
			CreatedAt: op.GetCreatedAt(),
		})
	}

	results := s.uc.SyncOperations(ctx, ops)

	reply := &v1.SyncOperationsReply{
		Results: make([]*v1.SyncOperationResult, 0, len(results)),
	}
	for _, r := range results {
		reply.Results = append(reply.Results, &v1.SyncOperationResult{
			Uuid:         r.UUID,
			Status:       r.Status,
			ErrorMessage: r.ErrorMessage,
		})
	}

	return reply, nil
}
