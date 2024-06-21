package rate

import (
	"log"
	"sync"
	"time"
)

const Window = time.Minute
const MaxRequests = 2400

type Limiter struct {
	maxRequests  int
	mu           sync.Mutex
	window       time.Duration
	requestTimes []time.Time
}

func NewLimiter(maxCount int, window time.Duration) *Limiter {
	return &Limiter{
		window:       time.Duration(window),
		maxRequests:  maxCount,
		requestTimes: []time.Time{},
	}
}

func (l *Limiter) Allow(weight int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)
	newRequestTimes := make([]time.Time, 0)
	for _, t := range l.requestTimes {
		if t.After(cutoff) {
			newRequestTimes = append(newRequestTimes, t)
		}
	}

	if len(newRequestTimes)+weight <= l.maxRequests {
		for i := 0; i < weight; i++ {
			l.requestTimes = append(l.requestTimes, now)
		}
		log.Println("RATELIMITER: %d", len(newRequestTimes))
		return true
	}
	return false
}
