package data

import (
	"context"

	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/ent/salereturn"
	"github.com/makesalekz/sales/ent/salereturnitem"

	"github.com/shopspring/decimal"
)

type ReturnItemDto struct {
	SaleItemID int64
	ProductID  int64
	Quantity   decimal.Decimal
	UnitPrice  decimal.Decimal
	Total      decimal.Decimal
}

type ReturnDto struct {
	UUID      string
	TenantID  int64
	SaleID    int64
	CashierID int64
	Total     decimal.Decimal
	Items     []ReturnItemDto
}

type ReturnsRepo interface {
	Create(ctx context.Context, dto ReturnDto) (*ent.SaleReturn, error)
	GetReturnedQuantities(ctx context.Context, saleID int64) (map[int64]decimal.Decimal, error)
	GetByUUID(ctx context.Context, uuid string) (*ent.SaleReturn, error)
}

type returnsRepo struct {
	db *ent.Client
}

func NewReturnsRepo(d *Data) ReturnsRepo {
	return &returnsRepo{db: d.db}
}

func (r *returnsRepo) Create(ctx context.Context, dto ReturnDto) (*ent.SaleReturn, error) {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return nil, err
	}

	create := tx.SaleReturn.Create().
		SetTenantID(dto.TenantID).
		SetSaleID(dto.SaleID).
		SetCashierID(dto.CashierID).
		SetTotal(dto.Total)
	if dto.UUID != "" {
		create = create.SetUUID(dto.UUID)
	}
	sr, err := create.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	for _, item := range dto.Items {
		_, err := tx.SaleReturnItem.Create().
			SetSaleReturnID(sr.ID).
			SetSaleItemID(item.SaleItemID).
			SetProductID(item.ProductID).
			SetQuantity(item.Quantity).
			SetUnitPrice(item.UnitPrice).
			SetTotal(item.Total).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.db.SaleReturn.Query().
		Where(salereturn.ID(sr.ID)).
		WithItems().
		Only(ctx)
}

func (r *returnsRepo) GetReturnedQuantities(ctx context.Context, saleID int64) (map[int64]decimal.Decimal, error) {
	// Get all return IDs for this sale
	returns, err := r.db.SaleReturn.Query().
		Where(salereturn.SaleID(saleID)).
		IDs(ctx)
	if err != nil {
		return nil, err
	}

	if len(returns) == 0 {
		return make(map[int64]decimal.Decimal), nil
	}

	// Get all return items for those returns
	items, err := r.db.SaleReturnItem.Query().
		Where(salereturnitem.SaleReturnIDIn(returns...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]decimal.Decimal)
	for _, item := range items {
		result[item.SaleItemID] = result[item.SaleItemID].Add(item.Quantity)
	}

	return result, nil
}

func (r *returnsRepo) GetByUUID(ctx context.Context, uuid string) (*ent.SaleReturn, error) {
	return r.db.SaleReturn.Query().
		Where(salereturn.UUID(uuid)).
		WithItems().
		Only(ctx)
}
