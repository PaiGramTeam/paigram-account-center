package server

import (
	"context"
	"log"
	"sync"
	"time"
)

const defaultArtifactCleanupInterval = 5 * time.Minute
const artifactCleanupTimeout = 30 * time.Second

type expiredArtifactCleaner interface {
	DeleteExpired(ctx context.Context, expiredBefore time.Time) (int64, error)
	RetryPending(ctx context.Context) error
}

type ArtifactCleanupServer struct {
	cleaner  expiredArtifactCleaner
	interval time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

func NewArtifactCleanupServer(cleaner expiredArtifactCleaner, interval time.Duration) *ArtifactCleanupServer {
	if interval <= 0 {
		interval = defaultArtifactCleanupInterval
	}
	return &ArtifactCleanupServer{cleaner: cleaner, interval: interval, stop: make(chan struct{})}
}

func (s *ArtifactCleanupServer) Start(ctx context.Context) error {
	s.cleanup(ctx, time.Now().UTC())
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stop:
			return nil
		case now := <-ticker.C:
			s.cleanup(ctx, now)
		}
	}
}

func (s *ArtifactCleanupServer) Stop(context.Context) error {
	s.stopOnce.Do(func() { close(s.stop) })
	return nil
}

func (s *ArtifactCleanupServer) cleanup(parent context.Context, expiredBefore time.Time) {
	ctx, cancel := context.WithTimeout(parent, artifactCleanupTimeout)
	defer cancel()
	if _, err := s.cleaner.DeleteExpired(ctx, expiredBefore.UTC()); err != nil {
		log.Printf("artifact cleanup failed: %v", err)
	}
	if err := s.cleaner.RetryPending(ctx); err != nil {
		log.Printf("artifact revocation retry failed: %v", err)
	}
}
