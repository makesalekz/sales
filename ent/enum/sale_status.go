package enum

type SaleStatus string

const (
	Completed SaleStatus = "COMPLETED"
	Returned  SaleStatus = "RETURNED"
)

func (s SaleStatus) Value() string {
	return string(s)
}

func (s SaleStatus) IsValid() bool {
	switch s {
	case Completed, Returned:
		return true
	}
	return false
}

func (SaleStatus) Values() []string {
	return []string{
		string(Completed),
		string(Returned),
	}
}
