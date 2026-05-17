# Story 3.4: Discounts (Скидки)

## Summary

Whole-sale discount support added to CreateSale. A cashier can now apply a discount on the entire check (PERCENTAGE or FIXED) on top of existing per-item discounts.

## Changes

### New files
- `ent/enum/discount_type.go` -- DiscountType enum (NONE, PERCENTAGE, FIXED)

### Modified files
- `ent/schema/sale.go` -- added `discount_type` (enum, default NONE) and `discount_value` (numeric, optional) fields
- `api/sales/v1/models.proto` -- added `discount_type` (field 11), `discount_value` (field 12) to Sale message (appended to preserve wire compat)
- `api/sales/v1/sales.proto` -- added `discount_type`, `discount_value` to CreateSaleRequest
- `internal/data/models.go` -- added DiscountType, DiscountValue to SaleDto
- `internal/data/sales.go` -- persist new fields in Create
- `internal/biz/sales.go` -- validation + calculation logic for whole-sale discount
- `internal/service/sales.go` -- parse request fields, map to reply; empty discount_type treated as NONE
- `internal/service/sales_test.go` -- 10 new tests for whole-sale discounts
- `internal/service/shifts_test.go` -- fixed pre-existing build error (undefined errShiftNotFound)

### Generated (via `make api` + `make ent`)
- `api/sales/v1/*.pb.go` -- regenerated from proto changes
- `ent/` -- regenerated from schema changes

## Design decisions

1. **Order of operations**: per-item discounts apply first (item_total = qty * price - item_discount), then whole-sale discount applies to the subtotal (sum of item totals).
2. **discount_total semantics**: includes both per-item and whole-sale discounts, so `gross - discount_total = total` holds.
3. **Percentage convention**: value 10 means 10%, computed as `subtotal * value / 100`.
4. **Backward compatibility**: empty `discount_type` from old clients maps to NONE. All 15 existing tests pass unchanged.
5. **Validation**: negative discount_value errors; percentage > 100 errors; fixed > subtotal errors.

## Tests added (10)

- TestCreateSale_PercentageDiscount -- 10% on subtotal 200 = total 180
- TestCreateSale_FixedDiscount -- fixed 50 on subtotal 200 = total 150
- TestCreateSale_PercentageWithPerItemDiscount -- combined per-item + whole-sale
- TestCreateSale_NoDiscountType_BackwardCompat -- old client (no fields) works
- TestCreateSale_ExplicitNoneDiscount -- explicit NONE works
- TestCreateSale_InvalidDiscountType -- rejects unknown type
- TestCreateSale_NegativeDiscountValue -- rejects negative
- TestCreateSale_PercentageOver100 -- rejects > 100%
- TestCreateSale_FixedExceedsSubtotal -- rejects fixed > subtotal
- TestCreateSale_PercentageDiscount_EventTotal -- NATS event reflects discounted total

## Status

All 44 tests pass (`go test ./internal/...`). Pre-existing `cmd/app` wire_gen.go issue (missing ReturnsServiceImpl) unrelated to this story.
