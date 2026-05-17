package enum

type PaymentType string

const (
	Cash  PaymentType = "CASH"
	Card  PaymentType = "CARD"
	Mixed PaymentType = "MIXED"
)

func (p PaymentType) Value() string {
	return string(p)
}

func (p PaymentType) IsValid() bool {
	switch p {
	case Cash, Card, Mixed:
		return true
	}
	return false
}

func (PaymentType) Values() []string {
	return []string{
		string(Cash),
		string(Card),
		string(Mixed),
	}
}
