package enum

type ShiftStatus string

const (
	ShiftOpen   ShiftStatus = "OPEN"
	ShiftClosed ShiftStatus = "CLOSED"
)

func (s ShiftStatus) Value() string {
	return string(s)
}

func (s ShiftStatus) IsValid() bool {
	switch s {
	case ShiftOpen, ShiftClosed:
		return true
	}
	return false
}

func (ShiftStatus) Values() []string {
	return []string{
		string(ShiftOpen),
		string(ShiftClosed),
	}
}
