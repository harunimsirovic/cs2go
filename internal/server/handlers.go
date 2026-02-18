package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"cs2-demo-analyzer/internal/models"
	"cs2-demo-analyzer/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	maxUploadSize = 500 << 20 // 500 MB
	uploadDir     = "uploads"
)

// Submitter is the interface the handlers use to enqueue jobs.
// Satisfied by *parser.Pipeline — avoids import cycles.
type Submitter interface {
	Submit(id, filePath string)
}

// Handlers holds dependencies for all HTTP handlers.
type Handlers struct {
	store    *storage.Store
	pipeline Submitter
}

func NewHandlers(store *storage.Store, pipeline Submitter) *Handlers {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("cannot create upload dir: %v", err)
	}
	return &Handlers{store: store, pipeline: pipeline}
}

// HandleUpload accepts a multipart/form-data POST with a "demo" file field.
//
// POST /upload
// Response: {"job_id": "<uuid>"}
func (h *Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		jsonError(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("demo")
	if err != nil {
		jsonError(w, "missing 'demo' field in form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Basic validation: check the file extension.
	if filepath.Ext(header.Filename) != ".dem" {
		jsonError(w, "only .dem files are accepted", http.StatusBadRequest)
		return
	}

	// Generate a unique job ID and save the file.
	jobID := uuid.New().String()
	destPath := filepath.Join(uploadDir, jobID+".dem")

	dst, err := os.Create(destPath)
	if err != nil {
		log.Printf("[upload] cannot create dest file: %v", err)
		jsonError(w, "internal error saving file", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(destPath)
		jsonError(w, "internal error writing file", http.StatusInternalServerError)
		return
	}
	dst.Close()

	log.Printf("[upload] saved %s → %s (%d bytes)", header.Filename, destPath, header.Size)

	// Register the job in the store, then hand it to the worker pool.
	h.store.CreateJob(jobID, destPath)
	h.pipeline.Submit(jobID, destPath)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"job_id":  jobID,
		"status":  "pending",
		"message": fmt.Sprintf("demo %q queued for processing", header.Filename),
	})
}

// HandleStatus returns the current status and progress of a job.
//
// GET /jobs/{jobID}
func (h *Handlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := h.store.GetJob(jobID)
	if err != nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	type statusResponse struct {
		JobID    string `json:"job_id"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
		Error    string `json:"error,omitempty"`
	}

	jsonOK(w, statusResponse{
		JobID:    job.ID,
		Status:   string(job.Status),
		Progress: job.Progress,
		Error:    job.Error,
	})
}

// HandleResult returns the full parsed stats once a job is complete.
// Also generates insights for each player.
//
// GET /jobs/{jobID}/result
func (h *Handlers) HandleResult(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := h.store.GetJob(jobID)
	if err != nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}

	if job.Status != storage.StatusDone {
		jsonError(w, fmt.Sprintf("job is not complete (status: %s)", job.Status), http.StatusConflict)
		return
	}

	// Attach insights to the result before returning.
	type playerWithInsights struct {
		*models.PlayerStats
		Insights []models.Insight `json:"insights"`
	}

	type enrichedResult struct {
		JobID    string                        `json:"job_id"`
		MapName  string                        `json:"map_name"`
		Duration float64                       `json:"duration_seconds"`
		Rounds   int                           `json:"total_rounds"`
		Players  map[uint64]playerWithInsights `json:"players"`
		RoundLog []models.RoundSnapshot        `json:"round_log"`
	}

	enriched := enrichedResult{
		JobID:    job.Result.JobID,
		MapName:  job.Result.MapName,
		Duration: job.Result.Duration,
		Rounds:   job.Result.Rounds,
		RoundLog: job.Result.RoundLog,

		Players: make(map[uint64]playerWithInsights),
	}

	for sid, stats := range job.Result.Players {
		enriched.Players[sid] = playerWithInsights{
			PlayerStats: stats,
			Insights:    models.GenerateInsights(stats),
		}
	}

	jsonOK(w, enriched)
}

// HandleListJobs returns all jobs (useful for a dashboard).
//
// GET /jobs
func (h *Handlers) HandleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := h.store.AllJobs()
	type jobSummary struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
	}
	out := make([]jobSummary, len(jobs))
	for i, j := range jobs {
		out[i] = jobSummary{ID: j.ID, Status: string(j.Status), Progress: j.Progress}
	}
	jsonOK(w, out)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
