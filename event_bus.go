package gobale

import (
	"log"
	"sync"
)

// EventListener is the callback signature receiving any dynamic payload
type EventListener func(payload any)

// EventBus handles central, topic-based event routing asynchronously
type EventBus struct {
	mu        sync.RWMutex
	listeners map[string][]EventListener
}

// NewEventBus instantiates a unified central event bus
func NewEventBus() *EventBus {
	return &EventBus{
		listeners: make(map[string][]EventListener),
	}
}

// Subscribe registers a listener callback to a specific topic
func (eb *EventBus) Subscribe(topic string, fn EventListener) {
	eb.mu.Lock()
	eb.listeners[topic] = append(eb.listeners[topic], fn)
	eb.mu.Unlock()
}

// Publish dispatches a payload concurrently to all listeners on a topic
func (eb *EventBus) Publish(topic string, payload any) {
	eb.mu.RLock()
	list, ok := eb.listeners[topic]
	eb.mu.RUnlock()

	if !ok || len(list) == 0 {
		return
	}

	// Spawn each listener in a concurrent, panic-proof goroutine
	for _, listener := range list {
		go func(l EventListener) {
			defer func() {
				if r := recover(); r != nil {
					// Catch and log listener panics to prevent the entire bot from crashing
					log.Printf("[EventBus Panic Recovery] Recovered from listener panic: %v", r)
				}
			}()
			l(payload)
		}(listener)
	}
}
