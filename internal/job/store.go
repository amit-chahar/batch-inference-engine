package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrJobNotFound is returned when a job directory or meta file does not exist.
var ErrJobNotFound = errors.New("job not found")

// Store persists job metadata and append-only results on disk.
// Layout per job:
//
//	{rootDir}/{jobID}/meta.json           — status + counters
//	{rootDir}/{jobID}/results.jsonl       — active chunk being appended
//	{rootDir}/{jobID}/chunks/chunk_N.jsonl — sealed chunks (optional rotation)
type Store struct {
	rootDir string
	locks   sync.Map
}

// NewStore creates a job store rooted at the given directory (typically data/jobs).
func NewStore(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

// CreateJob allocates a new job directory and writes initial metadata.
func (s *Store) CreateJob(totalItems int) (JobMeta, error) {
	return s.CreateJobWithOptions(JobCreateOptions{TotalItems: totalItems})
}

// CreateJobWithOptions allocates a job with optional webhook callback URL.
func (s *Store) CreateJobWithOptions(opts JobCreateOptions) (JobMeta, error) {
	jobID := uuid.NewString()
	dir := s.jobDir(jobID)

	if err := os.MkdirAll(filepath.Join(dir, "chunks"), 0o755); err != nil {
		return JobMeta{}, fmt.Errorf("create job dir: %w", err)
	}

	meta := JobMeta{
		JobID:       jobID,
		Status:      JobStatusPending,
		TotalItems:  opts.TotalItems,
		CreatedAt:   time.Now().UTC(),
		CallbackURL: strings.TrimSpace(opts.CallbackURL),
	}

	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	if err := s.writeMetaLocked(jobID, meta); err != nil {
		return JobMeta{}, err
	}

	if err := s.ensureActiveResultsFile(jobID); err != nil {
		return JobMeta{}, err
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

// AddChunkKey records an uploaded chunk object URL/key in job metadata.
func (s *Store) AddChunkKey(jobID, chunkURL string) error {
	return s.updateMeta(jobID, func(meta *JobMeta) {
		meta.ChunkKeys = append(meta.ChunkKeys, chunkURL)
	})
}

// AppendResult appends one result row without chunk rotation.
func (s *Store) AppendResult(jobID string, result PromptResult) error {
	_, err := s.AppendResultWithChunking(jobID, result, 0)
	return err
}

// AppendResultWithChunking appends a row and optionally seals a chunk at chunkSize lines.
func (s *Store) AppendResultWithChunking(jobID string, result PromptResult, chunkSize int) (*SealedChunk, error) {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	meta, err := s.readMetaLocked(jobID)
	if err != nil {
		return nil, err
	}

	line, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	resultsPath := s.activeResultsPath(jobID)
	file, err := os.OpenFile(resultsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open results file: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		return nil, fmt.Errorf("append result: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close results file: %w", err)
	}

	meta.ActiveResultLines++
	var sealed *SealedChunk
	if chunkSize > 0 && meta.ActiveResultLines >= chunkSize {
		sealed, err = s.sealActiveChunkLocked(jobID, &meta)
		if err != nil {
			return nil, err
		}
	}

	if err := s.writeMetaLocked(jobID, meta); err != nil {
		return nil, err
	}
	return sealed, nil
}

// SealActiveChunkIfNonEmpty seals the in-progress results file when the job finishes.
func (s *Store) SealActiveChunkIfNonEmpty(jobID string) (*SealedChunk, error) {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	meta, err := s.readMetaLocked(jobID)
	if err != nil {
		return nil, err
	}
	if meta.ActiveResultLines == 0 {
		return nil, nil
	}
	sealed, err := s.sealActiveChunkLocked(jobID, &meta)
	if err != nil {
		return nil, err
	}
	if err := s.writeMetaLocked(jobID, meta); err != nil {
		return nil, err
	}
	return sealed, nil
}

// StreamResults writes all job results (sealed chunks + active file) as a JSON array.
func (s *Store) StreamResults(jobID string, w io.Writer) error {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	if _, err := s.readMetaLocked(jobID); err != nil {
		return err
	}

	paths, err := s.resultFilePathsLocked(jobID)
	if err != nil {
		return err
	}
	return WriteResultsArrayFromFiles(w, paths...)
}

// ResultsPath returns the active JSONL results path for a job.
func (s *Store) ResultsPath(jobID string) (string, error) {
	mu := s.lock(jobID)
	mu.Lock()
	defer mu.Unlock()

	if _, err := s.readMetaLocked(jobID); err != nil {
		return "", err
	}
	return s.activeResultsPath(jobID), nil
}

func (s *Store) sealActiveChunkLocked(jobID string, meta *JobMeta) (*SealedChunk, error) {
	index := meta.NextChunkIndex
	chunkPath := s.chunkPath(jobID, index)
	activePath := s.activeResultsPath(jobID)

	if err := os.Rename(activePath, chunkPath); err != nil {
		return nil, fmt.Errorf("seal chunk: %w", err)
	}
	if err := s.ensureActiveResultsFile(jobID); err != nil {
		return nil, err
	}

	meta.NextChunkIndex++
	meta.ActiveResultLines = 0

	objectKey := fmt.Sprintf("%s/chunks/chunk_%d.jsonl", jobID, index)
	return &SealedChunk{
		LocalPath: chunkPath,
		Index:     index,
		ObjectKey: objectKey,
	}, nil
}

func (s *Store) resultFilePathsLocked(jobID string) ([]string, error) {
	chunksDir := filepath.Join(s.jobDir(jobID), "chunks")
	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return nil, fmt.Errorf("read chunks dir: %w", err)
	}

	var chunkNames []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "chunk_") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		chunkNames = append(chunkNames, entry.Name())
	}
	sort.Slice(chunkNames, func(i, j int) bool {
		left, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(chunkNames[i], "chunk_"), ".jsonl"))
		right, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(chunkNames[j], "chunk_"), ".jsonl"))
		return left < right
	})

	paths := make([]string, 0, len(chunkNames)+1)
	for _, name := range chunkNames {
		paths = append(paths, filepath.Join(chunksDir, name))
	}

	activePath := s.activeResultsPath(jobID)
	info, err := os.Stat(activePath)
	if err != nil {
		return nil, fmt.Errorf("stat active results: %w", err)
	}
	if info.Size() > 0 {
		paths = append(paths, activePath)
	}
	return paths, nil
}

func (s *Store) ensureActiveResultsFile(jobID string) error {
	path := s.activeResultsPath(jobID)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create results file: %w", err)
	}
	return file.Close()
}

func (s *Store) activeResultsPath(jobID string) string {
	return filepath.Join(s.jobDir(jobID), "results.jsonl")
}

func (s *Store) chunkPath(jobID string, index int) string {
	return filepath.Join(s.jobDir(jobID), "chunks", fmt.Sprintf("chunk_%d.jsonl", index))
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
