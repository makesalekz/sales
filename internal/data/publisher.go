package data

import (
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	nats "github.com/nats-io/nats.go"
)

const saleCompletedSubject = "sales.sale.completed"

type SaleCompletedEventItem struct {
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	Quantity    string `json:"quantity"`
	UnitPrice   string `json:"unit_price"`
	Discount    string `json:"discount"`
	Total       string `json:"total"`
}

type SaleCompletedEvent struct {
	TenantID  int64                    `json:"tenant_id"`
	SaleID    int64                    `json:"sale_id"`
	Total     string                   `json:"total"`
	Items     []SaleCompletedEventItem `json:"items"`
	Timestamp string                   `json:"timestamp"`
}

type SaleCompletedPublisher interface {
	Publish(event SaleCompletedEvent)
}

type natsSaleCompletedPublisher struct {
	nc  *nats.Conn
	log *log.Helper
}

func NewSaleCompletedPublisher(nc *nats.Conn, logger log.Logger) SaleCompletedPublisher {
	return &natsSaleCompletedPublisher{
		nc:  nc,
		log: log.NewHelper(logger),
	}
}

func (p *natsSaleCompletedPublisher) Publish(event SaleCompletedEvent) {
	if p.nc == nil {
		return
	}

	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		p.log.Errorf("failed to marshal sale completed event: %v", err)
		return
	}

	if err := p.nc.Publish(saleCompletedSubject, payload); err != nil {
		p.log.Errorf("failed to publish sale completed event: %v", err)
	}
}

// --- Return completed publisher ---

const returnCompletedSubject = "sales.return.completed"

type ReturnCompletedEventItem struct {
	ProductID  int64  `json:"product_id"`
	SaleItemID int64  `json:"sale_item_id"`
	Quantity   string `json:"quantity"`
	UnitPrice  string `json:"unit_price"`
	Total      string `json:"total"`
}

type ReturnCompletedEvent struct {
	TenantID int64                      `json:"tenant_id"`
	SaleID   int64                      `json:"sale_id"`
	ReturnID int64                      `json:"return_id"`
	Total    string                     `json:"total"`
	Items    []ReturnCompletedEventItem `json:"items"`
	Timestamp string                    `json:"timestamp"`
}

type ReturnCompletedPublisher interface {
	Publish(event ReturnCompletedEvent)
}

type natsReturnCompletedPublisher struct {
	nc  *nats.Conn
	log *log.Helper
}

func NewReturnCompletedPublisher(nc *nats.Conn, logger log.Logger) ReturnCompletedPublisher {
	return &natsReturnCompletedPublisher{
		nc:  nc,
		log: log.NewHelper(logger),
	}
}

func (p *natsReturnCompletedPublisher) Publish(event ReturnCompletedEvent) {
	if p.nc == nil {
		return
	}

	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		p.log.Errorf("failed to marshal return completed event: %v", err)
		return
	}

	if err := p.nc.Publish(returnCompletedSubject, payload); err != nil {
		p.log.Errorf("failed to publish return completed event: %v", err)
	}
}
