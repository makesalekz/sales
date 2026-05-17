package schema

import (
	"time"

	"gitlab.calendaria.team/services/sales/ent/enum"
	"gitlab.calendaria.team/services/sales/ent/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

type Shift struct {
	ent.Schema
}

func (Shift) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("tenant_id").Immutable(),
		field.Int64("cashier_id").Immutable(),
		field.Int64("store_id").Optional().Nillable(),
		field.Float("opening_amount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}),
		field.Float("closing_amount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("total_sales").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Float("total_returns").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Optional(),
		field.Enum("status").GoType(enum.ShiftStatus("")).Default(enum.ShiftOpen.Value()),
		field.Time("opened_at").Default(time.Now),
		field.Time("closed_at").Optional().Nillable(),
	}
}

func (Shift) Edges() []ent.Edge {
	return nil
}

func (Shift) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
		index.Fields("tenant_id", "cashier_id", "status"),
	}
}

func (Shift) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.CreateUpdateMixin{},
	}
}
