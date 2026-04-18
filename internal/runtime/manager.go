package runtime

import (
	"context"
	"sync"
)

type Starter interface {
	Start(context.Context) error
	Close() error
}

type AcceptorStopper interface {
	StopAccepting() error
}

type Factory func(context.Context, Item) (Starter, error)

type ApplyResult struct {
	Added   int
	Removed int
	Kept    int
}

type Status struct {
	State            string
	RuleCount        int
	TCPListenerCount int
	UDPListenerCount int
}

type Manager struct {
	mu      sync.RWMutex
	current Snapshot
	active  map[string]Starter
	state   string
}

func NewManager() *Manager {
	return &Manager{
		current: Snapshot{Items: map[string]Item{}},
		active:  make(map[string]Starter),
		state:   "running",
	}
}

func BuildApplyResult(diff DiffResult) ApplyResult {
	return ApplyResult{
		Added:   len(diff.Add),
		Removed: len(diff.Remove),
		Kept:    len(diff.Keep),
	}
}

func (m *Manager) Apply(ctx context.Context, next Snapshot, factory Factory) (ApplyResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	diff := Diff(m.current, next)

	for _, item := range diff.Remove {
		if active, ok := m.active[item.Key]; ok {
			_ = active.Close()
			delete(m.active, item.Key)
		}
	}

	for _, item := range diff.Add {
		if factory == nil {
			continue
		}

		active, err := factory(ctx, item)
		if err != nil {
			return ApplyResult{}, err
		}

		if err := active.Start(ctx); err != nil {
			return ApplyResult{}, err
		}

		m.active[item.Key] = active
	}

	m.current = next
	return BuildApplyResult(diff), nil
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := Status{
		State:     m.state,
		RuleCount: len(m.current.Items),
	}

	for _, item := range m.current.Items {
		switch item.Entry.Protocol {
		case "tcp":
			status.TCPListenerCount++
		case "udp":
			status.UDPListenerCount++
		}
	}

	return status
}

func (m *Manager) MarkStopping() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = "stopping"
}

func (m *Manager) StopAccepting() {
	for _, active := range m.snapshotActive() {
		if stopper, ok := active.(AcceptorStopper); ok {
			_ = stopper.StopAccepting()
		}
	}
}

func (m *Manager) CloseAll() {
	for _, active := range m.snapshotActive() {
		_ = active.Close()
	}
}

func (m *Manager) snapshotActive() []Starter {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]Starter, 0, len(m.active))
	for _, starter := range m.active {
		active = append(active, starter)
	}
	return active
}
