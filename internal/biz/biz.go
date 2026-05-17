package biz

import "github.com/google/wire"

var ProviderSet = wire.NewSet(NewSalesUsecase, NewShiftsUsecase, NewReturnsUsecase, NewCashierLogger, NewCashierLogUsecase, NewSyncUsecase)
