package enrich

import "sync"

// Budget meters estimated spend across a job. Reserve is all-or-nothing.
type Budget struct {
	mu    sync.Mutex
	limit float64
	spent float64
}

func NewBudget(limitEUR float64) *Budget { return &Budget{limit: limitEUR} }

// Reserve books cost if it fits. A zero-cost call always fits.
func (b *Budget) Reserve(cost float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cost > 0 && b.spent+cost > b.limit+1e-9 {
		return false
	}
	b.spent += cost
	return true
}

func (b *Budget) Spent() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

func (b *Budget) Limit() float64 { return b.limit }
