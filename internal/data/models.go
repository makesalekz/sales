package data

import (
	"github.com/shopspring/decimal"

	"gitlab.calendaria.team/services/sales/ent/enum"
)

type SaleItemDto struct {
	ProductID   int64
	ProductName string
	Quantity    decimal.Decimal
	UnitPrice   decimal.Decimal
	Discount    decimal.Decimal
	Total       decimal.Decimal
}

type SaleDto struct {
	UUID          string
	TenantID      int64
	ShiftID       int64
	CashierID     int64
	Total         decimal.Decimal
	DiscountTotal decimal.Decimal
	DiscountType  enum.DiscountType
	DiscountValue decimal.Decimal
	PaymentType   enum.PaymentType
	Status        enum.SaleStatus
	Items         []SaleItemDto
}
