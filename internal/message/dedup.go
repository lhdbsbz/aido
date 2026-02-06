package message

import (
	"sync"
	"time"
)

// Dedup prevents duplicate message processing using a TTL cache.
type Dedup struct {
	mu    sync.Mutex
	cache map[string]time.Time
	ttl   time.Duration
}

func NewDedup(ttl time.Duration) *Dedup {
	d := &Dedup{
		cache: make(map[string]time.Time),
		ttl:   ttl,
	}
	go d.cleanupLoop()
	return d
}

// IsDuplicate returns true if this message ID was seen recently.
// If not a duplicate, records it and returns false.
func (d *Dedup) IsDuplicate(key string) bool {
	if key == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.cache[key]; exists {
		return true
	}
	d.cache[key] = time.Now()
	return false
}

func (d *Dedup) cleanupLoop() {
	ticker := time.NewTicker(d.ttl)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		cutoff := time.Now().Add(-d.ttl)
		for k, t := range d.cache {
			if t.Before(cutoff) {
				delete(d.cache, k)
			}
		}
		d.mu.Unlock()
	}
}
