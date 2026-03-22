package scheduler

import "time"

type Subscription struct {
	CoinID   string
	Currency string
	Interval time.Duration
}
