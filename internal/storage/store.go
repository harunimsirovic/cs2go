package storage

import (
	"fmt"
	"sync"

	"cs2-demo-analyzer/internal/models"
)

// JobStatus tracks the lifecycle of a parse job.
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusDone       JobStatus = "done"
	StatusError      JobStatus = "error"
)

// Job represents a queued or completed parse job.
type Job struct {
	ID       string
	FilePath string
	Status   JobStatus
	Progress int // 0–100
	Error    string
	Result   *models.DemoResult
}

// Store is a thread-safe in-memory store for all jobs.
// In a real app you'd back this with Redis or a DB.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewStore() *Store {
	return &Store{
		jobs: make(map[string]*Job),
	}
}

// CreateJob registers a new job and returns it.
func (s *Store) CreateJob(id, filePath string) *Job {
	job := &Job{
		ID:       id,
		FilePath: filePath,
		Status:   StatusPending,
	}
	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	return job
}

// GetJob retrieves a job by ID. Returns an error if not found.
func (s *Store) GetJob(id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %q not found", id)
	}
	return job, nil
}

// UpdateProgress sets the progress percentage and marks the job as processing.
func (s *Store) UpdateProgress(id string, progress int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = StatusProcessing
		job.Progress = progress
	}
}

// SetResult marks the job as done and attaches the result.
func (s *Store) SetResult(id string, result *models.DemoResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = StatusDone
		job.Progress = 100
		job.Result = result
	}
}

// SetError marks the job as failed with an error message.
func (s *Store) SetError(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = StatusError
		job.Error = err.Error()
	}
}

// AllJobs returns a snapshot of all jobs (for a dashboard endpoint).
func (s *Store) AllJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out
}
