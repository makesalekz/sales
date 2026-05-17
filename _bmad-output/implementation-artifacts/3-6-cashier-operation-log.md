# Story 3.6: Cashier Operation Log

## Summary

Append-only audit log that records every cashier action (sale, return, shift open, shift close). Auto-created from existing operations via best-effort logging. New `GetCashierLog` RPC for querying with filtering and pagination.

## Changes

### New files
- `ent/enum/cashier_log_action.go` -- CashierLogAction enum (SALE, RETURN, SHIFT_OPEN, SHIFT_CLOSE, DISCOUNT, VOID). DISCOUNT/VOID are reserved for future stories, not auto-logged.
- `ent/schema/cashier_log.go` -- CashierLog schema: id, tenant_id, cashier_id, shift_id (optional), action (enum), entity_id, entity_type, amount (numeric), description, created_at. All fields immutable. No mixin (no updated_at) -- append-only.
- `internal/data/cashier_logs.go` -- CashierLogRepo interface + implementation (Create, List with filters)
- `internal/biz/cashier_log.go` -- CashierLogger interface (best-effort logging), CashierLogUsecase (GetCashierLog query)
- `internal/service/cashier_log.go` -- CashierLogServiceImpl with GetCashierLog RPC handler
- `internal/service/cashier_log_test.go` -- 13 new tests

### Modified files
- `api/sales/v1/models.proto` -- added CashierLogEntry message
- `api/sales/v1/sales.proto` -- added CashierLogService with GetCashierLog RPC, request/reply messages
- `internal/biz/sales.go` -- SalesUsecase now takes CashierLogger; logs SALE action after CreateSale
- `internal/biz/returns.go` -- ReturnsUsecase now takes CashierLogger; logs RETURN action after CreateReturn
- `internal/biz/shifts.go` -- ShiftsUsecase now takes CashierLogger; logs SHIFT_OPEN after OpenShift, SHIFT_CLOSE after CloseShift
- `internal/biz/biz.go` -- added NewCashierLogger, NewCashierLogUsecase to ProviderSet
- `internal/data/data.go` -- added NewCashierLogRepo to ProviderSet
- `internal/service/service.go` -- added NewCashierLogService to ProviderSet
- `internal/server/grpc.go` -- registered CashierLogServiceServer
- `internal/service/sync_test.go` -- renamed conflicting mock types to syncMockReturnsRepo/syncMockReturnPublisher

### Generated (via `make api` + `make ent` + `make generate`)
- `api/sales/v1/*.pb.go` -- regenerated
- `ent/` -- regenerated with CashierLog entity
- `cmd/app/wire_gen.go` -- regenerated

## Design decisions

1. **Append-only**: CashierLog schema has no updated_at, no soft-delete mixin, all fields immutable. No update/delete methods on repo.
2. **Best-effort logging**: CashierLogger.Log() writes to DB but errors are logged and swallowed -- matches the existing fire-and-forget publisher pattern. A sale/return/shift operation never fails due to a log write failure.
3. **CashierLogger as interface**: Injected into usecases as `biz.CashierLogger` interface. Tests pass a noop impl, keeping existing tests unaffected.
4. **DISCOUNT/VOID enum values**: Included in the enum for forward compatibility but not auto-logged. Reserved for future stories.
5. **Pagination**: Matches existing cursor-based convention (from_id + limit), same as ListShifts.
6. **All filters optional**: cashier_id, shift_id, date_from, date_to are all optional filters on GetCashierLog. tenant_id always applied from context.

## Test coverage (13 new tests)

- Auto-logging: sale creates SALE entry, return creates RETURN entry, open shift creates SHIFT_OPEN, close shift creates SHIFT_CLOSE
- GetCashierLog: happy path, filter by cashier, filter by shift, tenant isolation, no tenant error, pagination, invalid date formats, field mapping
- Resilience: sale succeeds even when log write fails
