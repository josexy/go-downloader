package download

import (
	"context"
	"sync"
	"time"
)

// bandwidth shares a small token bucket across all transfers. Settings changes
// wake waiters; no manager lock is held while waiting for network bandwidth.
type bandwidth struct {
	mu      sync.Mutex
	rate    int64
	tokens  float64
	updated time.Time
	changed chan struct{}
}

func (b *bandwidth) capacity() int {
	return int(max(1, min(16*1024, b.rate/10)))
}

func (b *bandwidth) refill(now time.Time) {
	if !b.updated.IsZero() && b.rate > 0 {
		b.tokens = min(float64(b.capacity()), b.tokens+now.Sub(b.updated).Seconds()*float64(b.rate))
	}
	b.updated = now
}

func (b *bandwidth) set(rate int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill(time.Now())
	previous := b.rate
	b.rate = max(0, rate)
	if previous == 0 {
		b.tokens = float64(b.capacity())
	} else {
		b.tokens = min(b.tokens, float64(b.capacity()))
	}
	if b.changed != nil {
		close(b.changed)
	}
	b.changed = make(chan struct{})
}

// take reserves bytes before reading. A short read refunds unused tokens.
func (b *bandwidth) take(ctx context.Context, want int) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		b.mu.Lock()
		if b.rate == 0 {
			b.mu.Unlock()
			return want, nil
		}
		b.refill(time.Now())
		n := min(want, b.capacity())
		if b.tokens >= float64(n) {
			b.tokens -= float64(n)
			b.mu.Unlock()
			return n, nil
		}
		wait := time.Duration((float64(n) - b.tokens) / float64(b.rate) * float64(time.Second))
		changed := b.changed
		b.mu.Unlock()
		timer := time.NewTimer(max(time.Millisecond, wait))
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (b *bandwidth) refund(n int) {
	if n <= 0 {
		return
	}
	b.mu.Lock()
	b.refill(time.Now())
	b.tokens = min(float64(b.capacity()), b.tokens+float64(n))
	b.mu.Unlock()
}

// SetRateLimit changes the total bytes per second allowed for all downloads.
// Zero or a negative value removes the limit, including for running transfers.
func (m *Manager) SetRateLimit(bytesPerSecond int64) {
	m.bandwidth.set(bytesPerSecond)
}
