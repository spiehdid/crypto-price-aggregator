package model

import (
	"time"

	"github.com/shopspring/decimal"
)

type AlertCondition int

const (
	ConditionAbove AlertCondition = iota
	ConditionBelow
)

func (c AlertCondition) String() string {
	switch c {
	case ConditionAbove:
		return "above"
	case ConditionBelow:
		return "below"
	default:
		return "unknown"
	}
}

type Alert struct {
	ID          string
	CoinID      string
	Currency    string
	Condition   AlertCondition
	Threshold   decimal.Decimal
	WebhookURL  string
	Active      bool
	CreatedAt   time.Time
	TriggeredAt *time.Time
}
