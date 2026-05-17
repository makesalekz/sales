package schema

import (
	"gitlab.calendaria.team/services/sales/ent/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

type SaleReturnItem struct {
	ent.Schema
}

func (SaleReturnItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("sale_return_id").Immutable(),
		field.Int64("sale_item_id").Immutable(),
		field.Int64("product_id").Immutable(),
		field.Float("quantity").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
		field.Float("unit_price").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
		field.Float("total").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
	}
}

func (SaleReturnItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("sale_return", SaleReturn.Type).
			Ref("items").
			Unique().
			Required().
			Immutable().
			Field("sale_return_id"),
	}
}

func (SaleReturnItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sale_return_id"),
		index.Fields("sale_item_id"),
	}
}

func (SaleReturnItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
