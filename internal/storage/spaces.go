package storage

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/amit-chahar/batch-inference-engine/internal/config"
)

// SpacesUploader uploads sealed chunk files to DigitalOcean Spaces (S3-compatible).
type SpacesUploader struct {
	client *s3.Client
	bucket string
	region string
}

// NewSpacesUploaderFromConfig builds an uploader when all Spaces env vars are set.
// Returns a disabled NoopUploader wrapper when credentials are incomplete.
func NewSpacesUploaderFromConfig(cfg config.Config) ChunkUploader {
	if cfg.SpacesKey == "" || cfg.SpacesSecret == "" || cfg.SpacesBucket == "" || cfg.SpacesRegion == "" {
		return NoopUploader{}
	}

	endpoint := fmt.Sprintf("https://%s.digitaloceanspaces.com", cfg.SpacesRegion)
	client := s3.New(s3.Options{
		Region: cfg.SpacesRegion,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
			cfg.SpacesKey,
			cfg.SpacesSecret,
			"",
		)),
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: false,
	})

	return &SpacesUploader{
		client: client,
		bucket: cfg.SpacesBucket,
		region: cfg.SpacesRegion,
	}
}

func (u *SpacesUploader) Enabled() bool { return u != nil && u.client != nil }

// UploadFile uploads a local chunk file and returns its public object URL.
func (u *SpacesUploader) UploadFile(ctx context.Context, objectKey, localPath string) (string, error) {
	if err := VerifyLocalFile(localPath); err != nil {
		return "", err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open chunk: %w", err)
	}
	defer file.Close()

	_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.bucket),
		Key:         aws.String(objectKey),
		Body:        file,
		ContentType: aws.String("application/x-ndjson"),
	})
	if err != nil {
		return "", fmt.Errorf("spaces put object: %w", err)
	}

	return fmt.Sprintf("https://%s.%s.digitaloceanspaces.com/%s", u.bucket, u.region, objectKey), nil
}
