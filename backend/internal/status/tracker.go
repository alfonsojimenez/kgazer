package status

import "sync"

type State string

const (
	Connecting   State = "connecting"
	Connected    State = "connected"
	Disconnected State = "disconnected"
)

type ClusterStatus struct {
	Name   string `json:"name"`
	Status State  `json:"status"`
	Paused bool   `json:"paused"`
}

type Tracker struct {
	mu       sync.RWMutex
	clusters map[string]State
	paused   map[string]bool
}

func NewTracker() *Tracker {
	return &Tracker{
		clusters: make(map[string]State),
		paused:   make(map[string]bool),
	}
}

func (t *Tracker) Set(name string, state State) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clusters[name] = state
}

func (t *Tracker) SetPaused(name string, paused bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.paused[name] = paused
}

func (t *Tracker) IsPaused(name string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.paused[name]
}

func (t *Tracker) ConfiguredClusterNames() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	names := make([]string, 0, len(t.clusters))
	for name := range t.clusters {
		names = append(names, name)
	}
	return names
}

func (t *Tracker) All() []ClusterStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]ClusterStatus, 0, len(t.clusters))
	for name, state := range t.clusters {
		result = append(result, ClusterStatus{
			Name:   name,
			Status: state,
			Paused: t.paused[name],
		})
	}
	return result
}
