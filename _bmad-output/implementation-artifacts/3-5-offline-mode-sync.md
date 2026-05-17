# Story 3.5: Offline-mode и синхронизация

## Summary

Batch offline sync endpoint added. Devices that operate offline accumulate SALE and RETURN operations locally, then upload them via `SyncOperations` RPC. Each operation carries a UUID for idempotency -- duplicates are detected and returned as `already_synced` without side effects.

## Changes

### New files
- `internal/biz/sync.go` -- SyncUsecase: iterates operations, checks UUID idempotency, deserializes protobuf data, delegates to CreateSale/CreateReturn
- `internal/service/sync.go` -- SyncServiceImpl: gRPC handler for SyncService
- `internal/service/sync_test.go` -- 12 tests covering sync scenarios

### Modified files
- `api/sales/v1/sales.proto` -- added SyncService RPC, SyncOperationType enum, SyncOperation/SyncOperationResult/SyncOperationsRequest/SyncOperationsReply messages
- `ent/schema/sale.go` -- added `uuid` field (string, optional, unique, nillable)
- `ent/schema/return.go` -- added `uuid` field (string, optional, unique, nillable)
- `internal/data/models.go` -- added UUID to SaleDto
- `internal/data/sales.go` -- added GetByUUID to SalesRepo interface; set UUID on Create; implemented GetByUUID
- `internal/data/returns.go` -- added UUID to ReturnDto; added GetByUUID to ReturnsRepo interface; set UUID on Create; implemented GetByUUID
- `internal/biz/sales.go` -- added UUID to CreateSaleInput; propagated to SaleDto
- `internal/biz/returns.go` -- added UUID to CreateReturnInput; propagated to ReturnDto
- `internal/biz/biz.go` -- added NewSyncUsecase to ProviderSet
- `internal/service/service.go` -- added NewSyncService to ProviderSet
- `internal/service/sales_test.go` -- updated mockSalesRepo.Create to set UUID on ent.Sale
- `internal/service/returns_test.go` -- updated mockReturnsRepo.Create to set UUID on ent.SaleReturn
- `internal/server/grpc.go` -- accepts SyncServiceImpl, registers SyncServiceServer
- `cmd/app/wire_gen.go` -- regenerated with sync wiring

### Generated (via `make api` + `go generate ./ent`)
- `api/sales/v1/*.pb.go` -- regenerated from proto changes
- `ent/` -- regenerated from schema changes (UUID field on Sale and SaleReturn)

## Design decisions

1. **Data format**: operation `data` bytes are protobuf-marshaled `CreateSaleRequest` / `CreateReturnRequest`. Reuses existing validation and keeps the offline client simple.
2. **Per-operation isolation**: each operation is independent. One failure does not affect others -- the RPC always returns OK with per-operation results.
3. **Idempotency**: UUID checked before processing (GetByUUID). If found, returns `already_synced`. On create failure (e.g. unique constraint race), re-checks UUID to handle concurrent syncs gracefully.
4. **Batch dedup**: duplicate UUIDs within the same batch are caught in-memory -- second occurrence returns `already_synced` without hitting the DB.
5. **UUID scope**: globally unique (DB unique constraint). No tenant scoping needed since UUIDs are collision-free by design.
6. **Tenant/cashier from context**: the authenticated device's identity (from gRPC metadata) is used, not from the operation payload.
7. **Nillable UUID**: existing sales/returns have no UUID (NULL). New field is Optional+Nillable to avoid backfill. Only sync-created records get a UUID set.

## Tests added (12)

- TestSync_NewSale_Synced -- happy path: single sale operation syncs
- TestSync_DuplicateUUID_AlreadySynced -- same UUID across two calls returns already_synced
- TestSync_DuplicateUUIDWithinBatch -- duplicate UUID in same batch: first synced, second already_synced
- TestSync_BatchPartialFailure -- bad data in middle, good ops on both sides succeed
- TestSync_InvalidProtoBytes_Error -- garbage bytes return error, no panic
- TestSync_EmptyUUID_Error -- empty UUID rejected
- TestSync_NoTenant_Error -- missing tenant returns gRPC error
- TestSync_EmptyOperations_Error -- empty operations list rejected
- TestSync_ReturnOperation -- return operation syncs correctly
- TestSync_ReturnDuplicate_AlreadySynced -- return idempotency works
- TestSync_MixedTypes -- batch with both SALE and RETURN operations
- TestSync_InvalidSaleData_OtherOpsSucceed -- invalid payment type in one op, others still sync

## Status

All 71 tests pass (`go test ./internal/...`). Clean build (`go build ./...`). Wire regenerated successfully.
