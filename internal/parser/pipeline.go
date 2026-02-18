package parser

import (
	"log"

	"cs2-demo-analyzer/internal/storage"
)

// Job is the unit of work sent to the worker pool.
type Job struct {
	ID       string
	FilePath string
}

// Pipeline manages a fixed pool of workers that process parse jobs concurrently.
// Each worker owns its own demoinfocs parser instance (parsers are not goroutine-safe).
type Pipeline struct {
	jobs    chan Job
	store   *storage.Store
	hub     ProgressHub
	workers int
}

// ProgressHub is satisfied by the WebSocket hub so the pipeline
// can push progress events without importing the server package (avoids cycles).
type ProgressHub interface {
	Broadcast(jobID string, msg any)
}

// ProgressUpdate is the JSON payload sent over WebSocket.
type ProgressUpdate struct {
	JobID    string `json:"job_id"`
	Progress int    `json:"progress"`
	Status   string `json:"status"`
}

// NewPipeline creates a pipeline and immediately starts `workerCount` goroutines.
// Workers block on the internal channel, waiting for jobs.
func NewPipeline(workerCount int, store *storage.Store, hub ProgressHub) *Pipeline {
	p := &Pipeline{
		// Buffer 100 pending jobs so Submit() doesn't block the HTTP handler.
		jobs:    make(chan Job, 100),
		store:   store,
		hub:     hub,
		workers: workerCount,
	}

	for i := range workerCount {
		workerID := i + 1
		go p.worker(workerID)
	}

	log.Printf("[pipeline] started %d workers", workerCount)
	return p
}

// Submit enqueues a job for processing. Non-blocking (buffered channel).
func (p *Pipeline) Submit(id, filePath string) {
	p.jobs <- Job{ID: id, FilePath: filePath}
	log.Printf("[pipeline] job %s submitted", id)
}

// worker is the goroutine that processes jobs from the channel.
// It loops forever, processing one job at a time, and blocks when idle.
func (p *Pipeline) worker(id int) {
	log.Printf("[worker %d] ready", id)

	for job := range p.jobs { // range over channel blocks until a job arrives
		log.Printf("[worker %d] starting job %s", id, job.ID)
		p.store.UpdateProgress(job.ID, 0)
		p.broadcast(job.ID, 0, "processing")

		// onProgress callback: called by the parser on each RoundEnd.
		// Safely updates the store and pushes a WS event.
		onProgress := func(progress int) {
			p.store.UpdateProgress(job.ID, progress)
			p.broadcast(job.ID, progress, "processing")
		}

		result, err := Parse(job.FilePath, onProgress)
		if err != nil {
			log.Printf("[worker %d] job %s failed: %v", id, job.ID, err)
			p.store.SetError(job.ID, err)
			p.broadcast(job.ID, 0, "error")
			continue
		}

		// Attach the job ID to the result for easy lookup.
		result.JobID = job.ID

		p.store.SetResult(job.ID, result)
		p.broadcast(job.ID, 100, "done")
		log.Printf("[worker %d] job %s complete (%d players, %d rounds)", id, job.ID, len(result.Players), result.Rounds)
	}
}

func (p *Pipeline) broadcast(jobID string, progress int, status string) {
	if p.hub != nil {
		p.hub.Broadcast(jobID, ProgressUpdate{
			JobID:    jobID,
			Progress: progress,
			Status:   status,
		})
	}
}
