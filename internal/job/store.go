package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrJobNotFound is returned when a job directory or meta file does not exist.
var ErrJobNotFound = errors.New("job not found")

// Store persists job metadata and append-only results on disk.
// Layout per job:
//
//	{rootDir}/{jobID}/meta.json      — status + counters
//	{rootDir}/{jobID}/results.jsonl  — one PromptResult JSON per line
type Store struct {
	rootDir string
	// locks provides one mutex per job ID so concurrent workers can append safely.
	locks sync.Map
}

// NewStore creates a job store rooted at the given directory (typically data/jobs).
func NewStore(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

// CreateJob allocates a new job directory and writes initial metadata.
// Returns the generated UUID so the API can respond immediately to POST /job/submit.
func (s *Store) CreateJob(totalItems int) (JobMeta, error) {
	jobID := uuid.NewString()
	dir := s.jobDir(jobID)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return JobMeta{}, fmt.Errorf("create job dir: %w", err)
	}

	meta := JobMeta{
		JobID:      jobID,
		Status:     JobStatusPending,
		TotalItems: totalItems,
		CreatedAt:  time.Now().UTC(),
	}

	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	if err := s.writeMetaLocked(jobID, meta); err != nil {
		return JobMeta{}, err
	}

	// Create an empty results file up front so append path is uniform.
	resultsPath := filepath.Join(dir, "results.jsonl")
	file, err := os.OpenFile(resultsPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return JobMeta{}, fmt.Errorf("create results file: %w", err)
	}
	if err := file.Close(); err != nil {
		return JobMeta{}, fmt.Errorf("close results file: %w", err)
	}

	return meta, nil
}

// GetMeta loads persisted metadata for a job.
func (s *Store) GetMeta(jobID string) (JobMeta, error) {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()
	return s.readMetaLocked(jobID)
}

// IncrementCompleted increases the completed item counter.
func (s *Store) IncrementCompleted(jobID string) error {
	return s.updateMeta(jobID, func(meta *JobMeta) {
		meta.CompletedItems++
	})
}

// IncrementFailed increases the failed item counter.
func (s *Store) IncrementFailed(jobID string) error {
	return s.updateMeta(jobID, func(meta *JobMeta) {
		meta.FailedItems++
	})
}

// SetStatus updates the job status (pending → running → completed/partial/failed).
func (s *Store) SetStatus(jobID string, status JobStatus) error {
	return s.updateMeta(jobID, func(meta *JobMeta) {
		meta.Status = status
	})
}

// AppendResult appends one result row to results.jsonl.
// Append-only JSONL avoids loading prior results into memory when adding new rows.
func (s *Store) AppendResult(jobID string, result PromptResult) error {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	if _, err := s.readMetaLocked(jobID); err != nil {
		return err
	}

	line, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	resultsPath := filepath.Join(s.jobDir(jobID), "results.jsonl")
	file, err := os.OpenFile(resultsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open results file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append result: %w", err)
	}

	return nil
}

// ResultsPath returns the persisted JSONL results path for a job.
func (s *Store) ResultsPath(jobID string) (string, error) {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	if _, err := s.readMetaLocked(jobID); err != nil {
		return "", err
	}
	return filepath.Join(s.jobDir(jobID), "results.jsonl"), nil
}

func (s *Store) updateMeta(jobID string, update func(*JobMeta)) error {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	meta, err := s.readMetaLocked(jobID)
	if err != nil {
		return err
	}

	update(&meta)
	return s.writeMetaLocked(jobID, meta)
}

func (s *Store) readMetaLocked(jobID string) (JobMeta, error) {
	path := filepath.Join(s.jobDir(jobID), "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return JobMeta{}, ErrJobNotFound
		}
		return JobMeta{}, fmt.Errorf("read meta: %w", err)
	}

	var meta JobMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return JobMeta{}, fmt.Errorf("decode meta: %w", err)
	}
	return meta, nil
}

// writeMetaLocked atomically replaces meta.json via temp file + rename.
func (s *Store) writeMetaLocked(jobID string, meta JobMeta) error {
	path := filepath.Join(s.jobDir(jobID), "meta.json")
	tmpPath := path + ".tmp"

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode meta: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write meta temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename meta: %w", err)
	}
	return nil
}

func (s *Store) jobDir(jobID string) string {
	return filepath.Join(s.rootDir, jobID)
}

func (s *Store) lock(jobID string) *sync.Mutex {
	value, _ := s.locks.LoadOrStore(jobID, &sync.Mutex{})
	return value.(*sync.Mutex)
}
