package message

import (
	"sync"
	"time"
)

// DebouncedMessage holds a pending message that might be merged with subsequent ones.
type DebouncedMessage struct {
	SenderKey string
	Text      string
	Callback  func(mergedText string)
}

// Debouncer batches rapid consecutive messages from the same sender.
type Debouncer struct {
	mu       sync.Mutex
	window   time.Duration
	pending  map[string]*debounceEntry
}

type debounceEntry struct {
	texts    []string
	timer    *time.Timer
	callback func(string)
}

func NewDebouncer(window time.Duration) *Debouncer {
	return &Debouncer{
		window:  window,
		pending: make(map[string]*debounceEntry),
	}
}

// Submit adds a message to the debounce buffer.
// When the debounce window expires without new messages, callback is called with merged text.
func (d *Debouncer) Submit(senderKey, text string, callback func(mergedText string)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, exists := d.pending[senderKey]
	if exists {
		entry.timer.Stop()
		entry.texts = append(entry.texts, text)
		entry.callback = callback
	} else {
		entry = &debounceEntry{
			texts:    []string{text},
			callback: callback,
		}
		d.pending[senderKey] = entry
	}

	entry.timer = time.AfterFunc(d.window, func() {
		d.mu.Lock()
		e, ok := d.pending[senderKey]
		if ok {
			delete(d.pending, senderKey)
		}
		d.mu.Unlock()

		if ok && e.callback != nil {
			merged := ""
			for i, t := range e.texts {
				if i > 0 {
					merged += "\n"
				}
				merged += t
			}
			e.callback(merged)
		}
	})
}
