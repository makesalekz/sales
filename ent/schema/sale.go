package schema

import (
	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/ent/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

type Sale struct {
	ent.Schema
}

func (Sale) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.String("uuid").Optional().Unique().Nillable(),
		field.Int64("tenant_id").Immutable(),
		field.Int64("shift_id").Optional().Default(0),
		field.Int64("cashier_id").Immutable(),
		field.Float("total").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
		field.Float("discount_total").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Enum("discount_type").GoType(enum.DiscountType("")).Default(enum.DiscountNone.Value()),
		field.Float("discount_value").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Enum("payment_type").GoType(enum.PaymentType("")).Default(enum.Cash.Value()),
		field.Enum("status").GoType(enum.SaleStatus("")).Default(enum.Completed.Value()),
	}
}

func (Sale) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", SaleItem.Type),
		edge.To("returns", SaleReturn.Type),
	}
}

func (Sale) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
		index.Fields("tenant_id", "shift_id"),
	}
}

func (Sale) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
