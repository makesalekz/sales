package schema

import (
	"github.com/makesalekz/sales/ent/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

type SaleItem struct {
	ent.Schema
}

func (SaleItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("sale_id").Immutable(),
		field.Int64("product_id").Immutable(),
		field.String("product_name"),
		field.Float("quantity").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
		field.Float("unit_price").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
		field.Float("discount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("total").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
	}
}

func (SaleItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("sale", Sale.Type).
			Ref("items").
			Unique().
			Required().
			Immutable().
			Field("sale_id"),
	}
}

func (SaleItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sale_id"),
	}
}

func (SaleItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
