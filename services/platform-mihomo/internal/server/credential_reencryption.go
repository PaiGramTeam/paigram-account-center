package server

import (
	"context"
	"log"
	"sync"
	"time"
)

type credentialReencryptor interface {
	ReencryptAll(context.Context) (int64, error)
}

type CredentialReencryptionServer struct {
	reencryptor credentialReencryptor
	interval    time.Duration
	stop        chan struct{}
	stopOnce    sync.Once
}

func NewCredentialReencryptionServer(reencryptor credentialReencryptor, interval time.Duration) *CredentialReencryptionServer {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &CredentialReencryptionServer{reencryptor: reencryptor, interval: interval, stop: make(chan struct{})}
}

func (s *CredentialReencryptionServer) Start(ctx context.Context) error {
	s.reencrypt(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stop:
			return nil
		case <-ticker.C:
			s.reencrypt(ctx)
		}
	}
}

func (s *CredentialReencryptionServer) Stop(context.Context) error {
	s.stopOnce.Do(func() { close(s.stop) })
	return nil
}

func (s *CredentialReencryptionServer) reencrypt(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	if _, err := s.reencryptor.ReencryptAll(ctx); err != nil {
		log.Printf("credential re-encryption failed: %v", err)
	}
}
