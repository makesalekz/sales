package enum

type CashierLogAction string

const (
	ActionSale       CashierLogAction = "SALE"
	ActionReturn     CashierLogAction = "RETURN"
	ActionShiftOpen  CashierLogAction = "SHIFT_OPEN"
	ActionShiftClose CashierLogAction = "SHIFT_CLOSE"
	ActionDiscount   CashierLogAction = "DISCOUNT"
	ActionVoid       CashierLogAction = "VOID"
)

func (a CashierLogAction) Value() string {
	return string(a)
}

func (a CashierLogAction) IsValid() bool {
	switch a {
	case ActionSale, ActionReturn, ActionShiftOpen, ActionShiftClose, ActionDiscount, ActionVoid:
		return true
	}
	return false
}

func (CashierLogAction) Values() []string {
	return []string{
		string(ActionSale),
		string(ActionReturn),
		string(ActionShiftOpen),
		string(ActionShiftClose),
		string(ActionDiscount),
		string(ActionVoid),
	}
}
