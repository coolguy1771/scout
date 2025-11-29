package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
)

type S3Storage struct {
	client *s3.Client
	bucket string
	logger *zap.Logger
}

func NewS3Storage(cfg config.S3Config, logger *zap.Logger) (*S3Storage, error) {
	var awsCfg aws.Config
	var err error

	if cfg.Endpoint != "" {
		// Custom endpoint (e.g., MinIO, LocalStack)
		awsCfg, err = awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(cfg.Region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			)),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}

		// Override endpoint
		awsCfg.BaseEndpoint = aws.String(cfg.Endpoint)
	} else {
		// Standard AWS S3
		awsCfg, err = awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(cfg.Region),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}

		// Use explicit credentials if provided
		if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
			awsCfg.Credentials = credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			)
		}
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.Endpoint != "" // Use path style for custom endpoints
	})

	return &S3Storage{
		client: client,
		bucket: cfg.Bucket,
		logger: logger,
	}, nil
}

// Upload uploads data to S3
func (s *S3Storage) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}
	return nil
}

// Download downloads data from S3
func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from S3: %w", err)
	}
	return result.Body, nil
}

// Delete deletes an object from S3
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}
	return nil
}

// GeneratePresignedURL generates a presigned URL for temporary access
func (s *S3Storage) GeneratePresignedURL(ctx context.Context, key string, duration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)
	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = duration
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return request.URL, nil
}

// Exists checks if an object exists
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if error indicates object not found
		if err, ok := err.(interface{ IsNotFound() bool }); ok && err.IsNotFound() {
			return false, nil
		}
		// For other errors, also return false (object doesn't exist)
		return false, nil
	}
	return true, nil
}

// ListObjects lists objects with a prefix
func (s *S3Storage) ListObjects(ctx context.Context, prefix string, maxKeys int32) ([]string, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	result, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(maxKeys),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	keys := make([]string, 0, len(result.Contents))
	for _, obj := range result.Contents {
		if obj.Key != nil {
			keys = append(keys, *obj.Key)
		}
	}

	return keys, nil
}

// HealthCheck checks if S3 storage is accessible
func (s *S3Storage) HealthCheck(ctx context.Context) error {
	// Try to list objects with a limit of 1 to verify connectivity
	_, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return fmt.Errorf("S3 health check failed: %w", err)
	}
	return nil
}
