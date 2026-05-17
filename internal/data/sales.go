package data

import (
	"context"

	"gitlab.calendaria.team/services/sales/ent"
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/ent/sale"
)

type SalesRepo interface {
	Create(ctx context.Context, dto SaleDto) (*ent.Sale, error)
	GetByIDWithItems(ctx context.Context, id, tenantID int64) (*ent.Sale, error)
	UpdateStatus(ctx context.Context, id int64, status enum.SaleStatus) error
	GetByUUID(ctx context.Context, uuid string) (*ent.Sale, error)
}

type salesRepo struct {
	db *ent.Client
}

func NewSalesRepo(d *Data) SalesRepo {
	return &salesRepo{db: d.db}
}

func (r *salesRepo) Create(ctx context.Context, dto SaleDto) (*ent.Sale, error) {
	tx, err := r.db.Tx(ctx)
	if err != nil {
		return nil, err
	}

	create := tx.Sale.Create().
		SetTenantID(dto.TenantID).
		SetShiftID(dto.ShiftID).
		SetCashierID(dto.CashierID).
		SetTotal(dto.Total).
		SetDiscountTotal(dto.DiscountTotal).
		SetDiscountType(dto.DiscountType).
		SetDiscountValue(dto.DiscountValue).
		SetPaymentType(dto.PaymentType).
		SetStatus(dto.Status)
	if dto.UUID != "" {
		create = create.SetUUID(dto.UUID)
	}
	s, err := create.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	for _, item := range dto.Items {
		_, err := tx.SaleItem.Create().
			SetSaleID(s.ID).
			SetProductID(item.ProductID).
			SetProductName(item.ProductName).
			SetQuantity(item.Quantity).
			SetUnitPrice(item.UnitPrice).
			SetDiscount(item.Discount).
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

	return r.db.Sale.Query().
		Where(sale.ID(s.ID)).
		WithItems().
		Only(ctx)
}

func (r *salesRepo) GetByIDWithItems(ctx context.Context, id, tenantID int64) (*ent.Sale, error) {
	return r.db.Sale.Query().
		Where(sale.ID(id), sale.TenantID(tenantID)).
		WithItems().
		Only(ctx)
}

func (r *salesRepo) UpdateStatus(ctx context.Context, id int64, status enum.SaleStatus) error {
	return r.db.Sale.UpdateOneID(id).
		SetStatus(status).
		Exec(ctx)
}

func (r *salesRepo) GetByUUID(ctx context.Context, uuid string) (*ent.Sale, error) {
	return r.db.Sale.Query().
		Where(sale.UUID(uuid)).
		WithItems().
		Only(ctx)
}
