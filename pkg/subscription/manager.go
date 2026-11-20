package subscription

import (
	"crypto/md5"
	"fmt"
	"sync"
	"time"

	"github.com/allani-ca/jetweir/pkg/protocol"
	"github.com/allani-ca/jetweir/pkg/types"
)

// Manager manages subscription pooling and deduplication.
type Manager struct {
	mu             sync.RWMutex
	subscriptions  map[string]*Subscription
	clientHandles  map[string][]string // clientID -> subscription IDs
	reuseCounts    map[string]int       // subscription ID -> number of clients
}

// Subscription represents a unique upstream subscription.
type Subscription struct {
	ID       string
	Filters  types.SubscriptionFilters
	Clients  map[string]bool // clientID -> connected
	Hash     string
	Created  int64
	Rejoined int64 // last time a client joined
}

// NewManager creates a new subscription manager.
func NewManager() *Manager {
	return &Manager{
		subscriptions: make(map[string]*Subscription),
		clientHandles: make(map[string][]string),
		reuseCounts:   make(map[string]int),
	}
}

// GetOrCreateSubscription gets an existing subscription matching the filters,
// or creates a new one if no exact match exists.
func (m *Manager) GetOrCreateSubscription(clientID string, filters types.SubscriptionFilters) *Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Try to find an exact matching subscription
	subID := m.findExactMatch(filters)
	if subID != "" {
		sub := m.subscriptions[subID]
		sub.Clients[clientID] = true
		m.clientHandles[clientID] = append(m.clientHandles[clientID], subID)
		m.reuseCounts[subID]++
		return sub
	}

	// Create a new subscription
	subID = generateSubscriptionID()
	sub := &Subscription{
		ID:      subID,
		Filters: filters,
		Clients: map[string]bool{clientID: true},
		Hash:    protocol.FilterHash(filters),
	}
	m.subscriptions[subID] = sub
	m.clientHandles[clientID] = append(m.clientHandles[clientID], subID)
	m.reuseCounts[subID] = 1

	return sub
}

// RemoveClient removes a client from all subscriptions.
// Returns subscription IDs that now have no clients.
func (m *Manager) RemoveClient(clientID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	subIDs := m.clientHandles[clientID]
	var orphaned []string

	for _, subID := range subIDs {
		if sub, exists := m.subscriptions[subID]; exists {
			delete(sub.Clients, clientID)
			m.reuseCounts[subID]--

			if len(sub.Clients) == 0 {
				orphaned = append(orphaned, subID)
				delete(m.subscriptions, subID)
				delete(m.reuseCounts, subID)
			}
		}
	}

	delete(m.clientHandles, clientID)
	return orphaned
}

// GetSubscription returns a subscription by ID.
func (m *Manager) GetSubscription(subID string) *Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subscriptions[subID]
}

// GetSubscriptionsForClient returns all subscriptions for a client.
func (m *Manager) GetSubscriptionsForClient(clientID string) []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subIDs := m.clientHandles[clientID]
	result := make([]*Subscription, 0, len(subIDs))
	for _, subID := range subIDs {
		if sub, exists := m.subscriptions[subID]; exists {
			result = append(result, sub)
		}
	}
	return result
}

// GetAllSubscriptions returns all active subscriptions.
func (m *Manager) GetAllSubscriptions() []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		result = append(result, sub)
	}
	return result
}

// GetStats returns subscription statistics.
func (m *Manager) GetStats() (count int, reuseMap map[string]int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	reuseMap = make(map[string]int)
	for id, count := range m.reuseCounts {
		reuseMap[id] = count
	}
	return len(m.subscriptions), reuseMap
}

// findExactMatch finds a subscription with identical filters.
func (m *Manager) findExactMatch(filters types.SubscriptionFilters) string {
	targetHash := protocol.FilterHash(filters)
	for _, sub := range m.subscriptions {
		if sub.Hash == targetHash {
			return sub.ID
		}
	}
	return ""
}

func generateSubscriptionID() string {
	h := md5.New()
	h.Write([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
