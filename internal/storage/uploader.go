package storage

import (
	"context"
	"fmt"
	"os"
)

// ChunkUploader uploads sealed result chunk files to external object storage.
// When disabled (missing credentials), the runner keeps local chunk files only.
type ChunkUploader interface {
	Enabled() bool
	UploadFile(ctx context.Context, objectKey, localPath string) (string, error)
}

// NoopUploader skips uploads — used when Spaces env vars are unset or in tests.
type NoopUploader struct{}

func (NoopUploader) Enabled() bool { return false }

func (NoopUploader) UploadFile(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// RecordingUploader is a test double that records uploads in memory.
type RecordingUploader struct {
	Uploads []RecordedUpload
}

type RecordedUpload struct {
	ObjectKey string
	LocalPath string
}

func (r *RecordingUploader) Enabled() bool { return true }

func (r *RecordingUploader) UploadFile(_ context.Context, objectKey, localPath string) (string, error) {
	r.Uploads = append(r.Uploads, RecordedUpload{ObjectKey: objectKey, LocalPath: localPath})
	return fmt.Sprintf("https://test.example.com/%s", objectKey), nil
}

// ObjectKeyForChunk builds the Spaces key for a sealed chunk file.
func ObjectKeyForChunk(jobID string, chunkIndex int) string {
	return fmt.Sprintf("%s/chunks/chunk_%d.jsonl", jobID, chunkIndex)
}

// VerifyLocalFile ensures the chunk exists before upload.
func VerifyLocalFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("chunk file is empty: %s", path)
	}
	return nil
}
