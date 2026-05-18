package schema

import (
	"time"

	"github.com/makesalekz/sales/ent/enum"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

type CashierLog struct {
	ent.Schema
}

func (CashierLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("tenant_id").Immutable(),
		field.Int64("cashier_id").Immutable(),
		field.Int64("shift_id").Optional().Default(0).Immutable(),
		field.Enum("action").GoType(enum.CashierLogAction("")).Immutable(),
		field.Int64("entity_id").Immutable(),
		field.String("entity_type").Immutable(),
		field.Float("amount").
			GoType(decimal.Decimal{}).
			SchemaType(map[string]string{dialect.Postgres: "numeric"}).
			Immutable(),
		field.String("description").Optional().Default("").Immutable(),
		field.Time("created_at").Immutable().Default(time.Now),
	}
}

func (CashierLog) Edges() []ent.Edge {
	return nil
}

func (CashierLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "cashier_id"),
		index.Fields("tenant_id", "shift_id"),
		index.Fields("tenant_id", "created_at"),
	}
}

func (CashierLog) Mixin() []ent.Mixin {
	return nil
}
