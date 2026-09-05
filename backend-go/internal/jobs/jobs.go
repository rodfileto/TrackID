// Package jobs is a small in-memory tracker for long-running background
// work that a client starts and then polls -- video processing being the
// first user. It knows nothing about what the work actually does; the
// result type is generic so this package stays reusable for any future
// "start it, poll it" endpoint.
package jobs

import (
	"sync"
	"time"
)

type Status string

const (
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

type job[T any] struct {
	mu             sync.Mutex
	status         Status
	frameCount     int
	expectedFrames int
	startedAt      time.Time
	finishedAt     time.Time
	result         T
	err            error
}

// Store holds every job for the life of the process -- there is no
// persistence and no eviction, so jobs disappear on restart and accumulate
// in memory for long-lived processes. Fine for now; worth revisiting if this
// ever needs to survive a restart or run for weeks without one.
type Store[T any] struct {
	mu   sync.RWMutex
	jobs map[string]*job[T]
}

func NewStore[T any]() *Store[T] {
	return &Store[T]{jobs: make(map[string]*job[T])}
}

func (s *Store[T]) Create(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[id] = &job[T]{status: StatusProcessing, startedAt: time.Now()}
}

func (s *Store[T]) UpdateProgress(id string, frameCount, expectedFrames int) {
	j, ok := s.get(id)
	if !ok {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.frameCount = frameCount
	j.expectedFrames = expectedFrames
}

func (s *Store[T]) Complete(id string, result T) {
	j, ok := s.get(id)
	if !ok {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = StatusCompleted
	j.result = result
	j.finishedAt = time.Now()
}

func (s *Store[T]) Fail(id string, err error) {
	j, ok := s.get(id)
	if !ok {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = StatusFailed
	j.err = err
	j.finishedAt = time.Now()
}

func (s *Store[T]) get(id string) (*job[T], bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

// Snapshot is a point-in-time, immutable copy of a job's state -- safe to
// hold onto and read after the lock is released.
type Snapshot[T any] struct {
	Status         Status
	FrameCount     int
	ExpectedFrames int
	Elapsed        time.Duration
	ETA            *time.Duration
	Err            error
	Result         T
}

func (s *Store[T]) Snapshot(id string) (Snapshot[T], bool) {
	j, ok := s.get(id)
	if !ok {
		return Snapshot[T]{}, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()

	end := time.Now()
	if !j.finishedAt.IsZero() {
		end = j.finishedAt
	}

	snap := Snapshot[T]{
		Status:         j.status,
		FrameCount:     j.frameCount,
		ExpectedFrames: j.expectedFrames,
		Elapsed:        end.Sub(j.startedAt),
		Err:            j.err,
		Result:         j.result,
	}

	if j.status == StatusProcessing && j.frameCount > 0 && j.expectedFrames > j.frameCount {
		rate := snap.Elapsed.Seconds() / float64(j.frameCount)
		remaining := time.Duration(rate*float64(j.expectedFrames-j.frameCount)) * time.Second
		snap.ETA = &remaining
	}

	return snap, true
}
