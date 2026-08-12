package servicehealth

import (
	"context"
	"errors"
	"sync"
	"time"
)

const defaultRefreshInterval = 2 * time.Second

// Publisher receives readiness transitions and the irreversible shutdown signal.
type Publisher interface {
	SetReady(bool)
	Shutdown()
}

// Monitor periodically projects a Checker onto a transport health publisher.
type Monitor struct {
	readiness Checker
	publisher Publisher
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	stateMu   sync.Mutex
	stopped   bool
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewMonitor(readiness Checker, publisher Publisher, interval time.Duration) *Monitor {
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Monitor{
		readiness: readiness,
		publisher: publisher,
		interval:  interval,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
}

func (m *Monitor) Refresh(ctx context.Context) error {
	if m == nil || m.readiness == nil || m.publisher == nil {
		return errors.New("readiness checker and publisher are required")
	}
	err := m.readiness.Check(ctx)

	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if m.stopped {
		return ErrShuttingDown
	}
	m.publisher.SetReady(err == nil)
	return err
}

func (m *Monitor) Start() {
	if m == nil {
		return
	}
	m.startOnce.Do(func() {
		_ = m.Refresh(m.ctx)
		go m.run()
	})
}

func (m *Monitor) run() {
	defer close(m.done)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			_ = m.Refresh(m.ctx)
		}
	}
}

func (m *Monitor) Shutdown() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		m.stateMu.Lock()
		m.stopped = true
		if m.publisher != nil {
			m.publisher.Shutdown()
		}
		m.stateMu.Unlock()

		m.cancel()
		m.startOnce.Do(func() { close(m.done) })
		<-m.done
	})
}
