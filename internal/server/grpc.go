package server

import (
	v1 "github.com/makesalekz/sales/api/sales/v1"
	"github.com/makesalekz/sales/internal/conf"
	"github.com/makesalekz/sales/internal/service"

	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

func NewGRPCServer(
	c *conf.Bootstrap,
	salesService *service.SalesService,
	shiftService *service.ShiftServiceImpl,
	returnsService *service.ReturnsServiceImpl,
	cashierLogService *service.CashierLogServiceImpl,
	syncService *service.SyncServiceImpl,
) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			metadata.Server(),
		),
	}
	if c.GetServer().GetGrpc().GetAddr() != "" {
		opts = append(opts, grpc.Address(c.GetServer().GetGrpc().GetAddr()))
	}
	if c.GetServer().GetGrpc().GetTimeout() != nil {
		opts = append(opts, grpc.Timeout(c.GetServer().GetGrpc().GetTimeout().AsDuration()))
	}
	srv := grpc.NewServer(opts...)

	v1.RegisterSalesServiceServer(srv, salesService)
	v1.RegisterShiftServiceServer(srv, shiftService)
	v1.RegisterReturnsServiceServer(srv, returnsService)
	v1.RegisterCashierLogServiceServer(srv, cashierLogService)
	v1.RegisterSyncServiceServer(srv, syncService)

	return srv
}
