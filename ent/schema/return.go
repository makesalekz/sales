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

type SaleReturn struct {
	ent.Schema
}

func (SaleReturn) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.String("uuid").Optional().Unique().Nillable(),
		field.Int64("tenant_id").Immutable(),
		field.Int64("sale_id").Immutable(),
		field.Int64("cashier_id").Immutable(),
		field.Float("total").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
	}
}

func (SaleReturn) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("sale", Sale.Type).
			Ref("returns").
			Unique().
			Required().
			Immutable().
			Field("sale_id"),
		edge.To("items", SaleReturnItem.Type),
	}
}

func (SaleReturn) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
		index.Fields("sale_id"),
	}
}

func (SaleReturn) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
