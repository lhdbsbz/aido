package message

import (
	"context"
	"sync"
)

// QueueMode determines how messages are handled when the agent is busy.
const (
	ModeCollect  = "collect"  // collect messages, respond once after quiet period
	ModeFollowup = "followup" // queue messages, process each after current run
	ModeSteer    = "steer"    // abort current, process newest
)

// QueueItem is a pending message in the queue.
type QueueItem struct {
	SenderKey string
	Text      string
	Done      chan string // receives the agent response
	Err       chan error
}

// Queue manages per-session message queuing.
type Queue struct {
	mu       sync.Mutex
	mode     string
	items    []*QueueItem
	active   bool // is an agent run currently in progress?
	cancelFn context.CancelFunc
}

func NewQueue(mode string) *Queue {
	if mode == "" {
		mode = ModeFollowup
	}
	return &Queue{mode: mode}
}

// Enqueue adds a message to the queue.
// Returns a channel that will receive the response.
func (q *Queue) Enqueue(senderKey, text string) (done chan string, errCh chan error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item := &QueueItem{
		SenderKey: senderKey,
		Text:      text,
		Done:      make(chan string, 1),
		Err:       make(chan error, 1),
	}

	switch q.mode {
	case ModeSteer:
		// Cancel current run and replace queue with this message
		if q.cancelFn != nil {
			q.cancelFn()
		}
		q.items = []*QueueItem{item}
	case ModeCollect:
		// Merge into queue, will be collected as one
		q.items = append(q.items, item)
	default: // followup
		q.items = append(q.items, item)
	}

	return item.Done, item.Err
}

// Next returns the next item to process, or nil if empty.
// For "collect" mode, merges all items into one.
func (q *Queue) Next() *QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	if q.mode == ModeCollect && len(q.items) > 1 {
		merged := ""
		for i, item := range q.items {
			if i > 0 {
				merged += "\n"
			}
			merged += item.Text
		}
		first := q.items[0]
		first.Text = merged
		// All items share the first item's response channels
		for _, item := range q.items[1:] {
			go func(it *QueueItem) {
				select {
				case resp := <-first.Done:
					first.Done <- resp // put back
					it.Done <- resp
				}
			}(item)
		}
		q.items = nil
		return first
	}

	item := q.items[0]
	q.items = q.items[1:]
	return item
}

// SetActive marks whether an agent run is in progress.
func (q *Queue) SetActive(active bool, cancelFn context.CancelFunc) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.active = active
	q.cancelFn = cancelFn
}

// IsActive returns whether an agent run is in progress.
func (q *Queue) IsActive() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.active
}

// Len returns the number of pending items.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
