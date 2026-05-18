package data

import (
	"context"
	"os"

	"github.com/makesalekz/sales/ent"
	"github.com/makesalekz/sales/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	_ "github.com/lib/pq"

	_ "github.com/makesalekz/sales/ent/runtime"
)

var ProviderSet = wire.NewSet(
	NewData,
	NewNatsClient,
	NewSalesRepo,
	NewSaleCompletedPublisher,
	NewReturnsRepo,
	NewReturnCompletedPublisher,
	NewShiftsRepo,
	NewCashierLogRepo,
)

type Data struct {
	db *ent.Client
}

func NewData(bc *conf.Bootstrap, logger log.Logger) (*Data, func(), error) {
	l := log.NewHelper(logger)

	dbDsn := bc.GetDb()
	if dbDsn == "" {
		l.Fatal("db dsn not configured")
		return nil, nil, nil
	}

	autoMigrate := os.Getenv("AUTOMIGRATE")
	entLogging := os.Getenv("ENT_LOGGING")
	var options []ent.Option
	if entLogging == "true" {
		options = append(options, ent.Debug(), ent.Log(l.Info))
	}

	client, err := ent.Open("postgres", dbDsn, options...)
	if err != nil {
		l.Fatalf("failed opening connection to postgres: %v", err)
		return nil, nil, err
	}

	if autoMigrate != "" {
		if err2 := client.Schema.Create(context.Background()); err2 != nil {
			l.Errorf("failed creating schema resources: %v", err2)
			return nil, nil, err2
		}
	}

	l.Info("Connected to postgres")

	cleanup := func() {
		if err2 := client.Close(); err2 != nil {
			l.Error(err2)
		}
	}

	return &Data{db: client}, cleanup, nil
}
