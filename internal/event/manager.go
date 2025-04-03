// internal/event/manager.go
package event

import (
	"sync"

	"github.com/bethropolis/tide/internal/logger"
)

// Handler defines the function signature for event subscribers.
// It returns true if the event was consumed (prevents further processing if needed).
// For now, we won't use the return value, but it's good practice for future flexibility.
type Handler func(e Event) bool

// Manager handles event subscriptions and dispatching.
type Manager struct {
	mu       sync.RWMutex
	handlers map[Type][]Handler // Map event types to a list of handlers
}

// NewManager creates a new event manager.
func NewManager() *Manager {
	return &Manager{
		handlers: make(map[Type][]Handler),
	}
}

// Subscribe adds a handler function for a specific event type.
func (m *Manager) Subscribe(eventType Type, handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handlers[eventType] = append(m.handlers[eventType], handler)
	logger.Debugf("Event Manager: Handler subscribed to type %v", eventType) // Debug log
}

// Unsubscribe (Optional): Removes a specific handler. Requires comparing function pointers, which can be tricky.
// A simpler approach might be to have Subscribe return an ID that can be used to Unsubscribe.
// Skipping implementation for now for brevity.

// Dispatch sends an event to all registered handlers for its type.
// Runs handlers synchronously for simplicity. Could be made asynchronous.
func (m *Manager) Dispatch(eventType Type, data interface{}) {
	event := Event{
		Type: eventType,
		Data: data,
	}

	m.mu.RLock() // Use read lock while iterating handlers
	handlers, exists := m.handlers[eventType]
	m.mu.RUnlock() // Unlock after getting the slice

	if !exists || len(handlers) == 0 {
		logger.Debugf("Event Manager: No handlers for type %v", eventType) // Can be noisy
		return
	}

	logger.Debugf("Event Manager: Dispatching event type %v to %d handler(s)", eventType, len(handlers)) // Debug log

	// Call handlers. Be careful if handlers can modify the list concurrently (they shouldn't).
	// Creating a copy prevents issues if a handler tries to unsubscribe itself during dispatch.
	handlersCopy := make([]Handler, len(handlers))
	copy(handlersCopy, handlers)

	for _, handler := range handlersCopy {
		// We could use the return value later if needed:
		// consumed := handler(event)
		// if consumed { break } // Stop propagation if consumed
		handler(event) // Simple synchronous dispatch
	}
}
