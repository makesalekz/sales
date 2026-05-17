# Story 3.2: Product Return (Возврат товара)

## Status: DONE

## Summary

Implemented the CreateReturn RPC for partial and full product returns. A cashier can return items from a completed sale by specifying sale_item_id + quantity. The system validates return quantities against original sold quantities minus already-returned quantities, computes totals using the original unit_price, and publishes a NATS event with a negative total for billing.

## Changes

### Proto (api/sales/v1/)
- `models.proto`: Added `Return` and `ReturnItem` messages
- `sales.proto`: Added `ReturnsService` with `CreateReturn` RPC, plus `CreateReturnRequest`, `CreateReturnItemRequest`, `CreateReturnReply` messages

### Ent Schemas (ent/schema/)
- `return.go`: `SaleReturn` schema (id, tenant_id, sale_id FK, cashier_id, total decimal, created_at/updated_at)
- `return_item.go`: `SaleReturnItem` schema (id, sale_return_id FK, sale_item_id, product_id, quantity decimal, unit_price decimal, total decimal)
- `sale.go`: Added `returns` edge from Sale to SaleReturn

Note: Schema type is `SaleReturn` / `SaleReturnItem` because `Return` is a Go reserved keyword.

### Data Layer (internal/data/)
- `returns.go`: `ReturnsRepo` interface with `Create` (transactional) and `GetReturnedQuantities` (aggregates returned qty per sale_item_id across all returns for a sale)
- `publisher.go`: Added `ReturnCompletedPublisher` interface + NATS implementation publishing to `sales.return.completed`
- `sales.go`: Extended `SalesRepo` with `GetByIDWithItems` and `UpdateStatus` methods
- `data.go`: Added `NewReturnsRepo` and `NewReturnCompletedPublisher` to ProviderSet

### Business Logic (internal/biz/)
- `returns.go`: `ReturnsUsecase.CreateReturn` - validates, computes, creates return, updates sale status if fully returned, publishes event
- `biz.go`: Added `NewReturnsUsecase` to ProviderSet

### Service Layer (internal/service/)
- `returns.go`: `ReturnsServiceImpl` implementing gRPC `CreateReturn` handler
- `service.go`: Added `NewReturnsService` to ProviderSet

### Server (internal/server/)
- `grpc.go`: Registered `ReturnsService` in gRPC server

### Wire (cmd/app/)
- `wire_gen.go`: Regenerated to include returns dependencies

## Validation Rules
1. Sale must exist and belong to the requesting tenant
2. Sale must not be already RETURNED (fully returned)
3. Each sale_item_id must belong to the specified sale
4. Return quantity must be > 0
5. Return quantity + already returned quantity <= original sold quantity (including duplicate sale_item_ids within the same request)

## Behavior
- **Partial return**: Sale stays COMPLETED. Only the returned items/quantities are recorded.
- **Full return**: When all items are fully returned (across one or more returns), Sale.status flips to RETURNED.
- **NATS event**: Published to `sales.return.completed` with negative total (for billing deduction).
- **Return.total**: Stored as positive value. Only negated in the NATS event payload.
- **unit_price**: Copied from the original SaleItem (historical price preservation).

## Tests (15 new tests)
- `TestCreateReturn_FullReturn` - full return marks sale RETURNED, event has negative total
- `TestCreateReturn_PartialReturn` - partial return keeps sale COMPLETED
- `TestCreateReturn_TwoPartialReturnsSumToFull` - second partial return flips sale to RETURNED
- `TestCreateReturn_OverReturnRejected` - quantity > remaining is rejected
- `TestCreateReturn_AlreadyFullyReturned` - cannot return already-returned sale
- `TestCreateReturn_SaleItemBelongsToDifferentSale` - cross-sale item rejected
- `TestCreateReturn_TenantIsolation` - cannot return another tenant's sale
- `TestCreateReturn_NoTenant` - missing tenant rejected
- `TestCreateReturn_EmptyItems` - empty items rejected
- `TestCreateReturn_ZeroQuantityRejected` - zero quantity rejected
- `TestCreateReturn_SaleNotFound` - nonexistent sale rejected
- `TestCreateReturn_DecimalPrecision` - decimal math preserved
- `TestCreateReturn_EventItemFields` - NATS event fields verified
- `TestCreateReturn_RepoError_NoEventPublished` - repo error prevents event
- `TestCreateReturn_DuplicateSaleItemIdOverReturnRejected` - duplicate sale_item_id in request cannot bypass qty validation

## Known Limitations
- Concurrent return requests (TOCTOU): `GetReturnedQuantities` runs outside the create transaction, so two simultaneous return requests could both pass validation. Standard POS race condition; not addressed here.
- Shift summary (`GetShiftSalesSummary`) derives `total_returns` from `Sale.status=RETURNED` rows. For partial returns the sale stays COMPLETED, so partial return amounts are not reflected in shift totals until the sale is fully returned. This is a follow-up item.
