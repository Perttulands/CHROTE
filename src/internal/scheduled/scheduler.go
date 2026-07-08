package scheduled

import (
	"context"
	"log"
	"sync"
	"time"
)

const defaultSchedulerInterval = 30 * time.Second

// Scheduler periodically reloads persisted tasks and asks Service to fire due work.
type Scheduler struct {
	service  *Service
	interval time.Duration

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	done    chan struct{}
}

// NewScheduler creates a scheduled-task runner. The scheduler is idle until Start
// or Tick is called.
func NewScheduler(service *Service, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = defaultSchedulerInterval
	}
	return &Scheduler{service: service, interval: interval}
}

// Start begins the background scheduler loop.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	s.running = true
	stop := s.stop
	done := s.done
	s.mu.Unlock()

	go func() {
		defer close(done)
		if err := s.Tick(context.Background()); err != nil {
			log.Printf("Warning: scheduled task tick failed: %v", err)
		}
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.Tick(context.Background()); err != nil {
					log.Printf("Warning: scheduled task tick failed: %v", err)
				}
			case <-stop:
				return
			}
		}
	}()
	return nil
}

// Stop terminates the background scheduler loop.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	stop := s.stop
	done := s.done
	s.running = false
	s.stop = nil
	s.done = nil
	close(stop)
	s.mu.Unlock()
	<-done
}

// Tick reloads tasks, computes missing nextRun values, and fires due tasks once.
func (s *Scheduler) Tick(ctx context.Context) error {
	if s == nil || s.service == nil {
		return nil
	}
	_, err := s.service.RunDue(ctx)
	return err
}
