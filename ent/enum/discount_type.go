package enum

type DiscountType string

const (
	DiscountNone       DiscountType = "NONE"
	DiscountPercentage DiscountType = "PERCENTAGE"
	DiscountFixed      DiscountType = "FIXED"
)

func (d DiscountType) Value() string {
	return string(d)
}

func (d DiscountType) IsValid() bool {
	switch d {
	case DiscountNone, DiscountPercentage, DiscountFixed:
		return true
	}
	return false
}

func (DiscountType) Values() []string {
	return []string{
		string(DiscountNone),
		string(DiscountPercentage),
		string(DiscountFixed),
	}
}
