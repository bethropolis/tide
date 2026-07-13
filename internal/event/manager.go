package event

import (
	"sync"
	"sync/atomic"

	"github.com/bethropolis/tide/internal/logger"
)

type Handler func(e Event) bool

type SubscriptionID uint64

type subscriptionEntry struct {
	id      SubscriptionID
	handler Handler
}

type Manager struct {
	mu         sync.RWMutex
	nextID     uint64
	handlers   map[Type][]subscriptionEntry
}

func NewManager() *Manager {
	return &Manager{
		handlers: make(map[Type][]subscriptionEntry),
	}
}

func (m *Manager) Subscribe(eventType Type, handler Handler) SubscriptionID {
	id := SubscriptionID(atomic.AddUint64(&m.nextID, 1))

	m.mu.Lock()
	m.handlers[eventType] = append(m.handlers[eventType], subscriptionEntry{id: id, handler: handler})
	m.mu.Unlock()

	logger.DebugTagf("event", "Event Manager: Handler subscribed to type %v (id=%d)", eventType, id)
	return id
}

func (m *Manager) Unsubscribe(eventType Type, id SubscriptionID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, exists := m.handlers[eventType]
	if !exists {
		return
	}

	for i, entry := range entries {
		if entry.id == id {
			m.handlers[eventType] = append(entries[:i], entries[i+1:]...)
			logger.DebugTagf("event", "Event Manager: Handler unsubscribed from type %v (id=%d)", eventType, id)
			return
		}
	}
}

func (m *Manager) Dispatch(eventType Type, data interface{}) {
	event := Event{
		Type: eventType,
		Data: data,
	}

	m.mu.RLock()
	entries, exists := m.handlers[eventType]
	m.mu.RUnlock()

	if !exists || len(entries) == 0 {
		logger.DebugTagf("event", "Event Manager: No handlers for type %v", eventType)
		return
	}

	logger.DebugTagf("event", "Event Manager: Dispatching event type %v to %d handler(s)", eventType, len(entries))

	handlersCopy := make([]Handler, len(entries))
	for i, entry := range entries {
		handlersCopy[i] = entry.handler
	}

	for _, handler := range handlersCopy {
		handler(event)
	}
}
