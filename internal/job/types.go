package job

import "time"

// JobStatus represents the lifecycle state of a batch job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusPartial   JobStatus = "partial"
)

// PromptItem is a single input row from the batch file.
type PromptItem struct {
	ID       string         `json:"id"`
	Prompt   string         `json:"prompt"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// PromptResult is the inference outcome for one prompt row.
type PromptResult struct {
	ID       string         `json:"id"`
	Prompt   string         `json:"prompt"`
	Response *string        `json:"response,omitempty"`
	Error    *string        `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// JobMeta tracks persisted job metadata on disk.
type JobMeta struct {
	JobID          string    `json:"job_id"`
	Status         JobStatus `json:"status"`
	TotalItems     int       `json:"total_items"`
	CompletedItems int       `json:"completed_items"`
	FailedItems    int       `json:"failed_items"`
	CreatedAt      time.Time `json:"created_at"`
}

// ProgressPercent returns completed progress as a percentage of total items.
func (m JobMeta) ProgressPercent() float64 {
	if m.TotalItems == 0 {
		return 0
	}
	return float64(m.CompletedItems+m.FailedItems) / float64(m.TotalItems) * 100
}
