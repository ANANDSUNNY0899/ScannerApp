package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type StorageService interface {
	Upload(ctx context.Context, bucket string, key string, file io.Reader, size int64, contentType string) (string, error)
	GetDownloadURL(ctx context.Context, bucket string, key string) (string, error)
	EnsureBucket(ctx context.Context, bucket string) error
}

type S3Storage struct {
	client *minio.Client
}

func NewS3Storage(endpoint, accessKey, secretKey string, useSSL bool) (*S3Storage, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize minio client: %w", err)
	}

	return &S3Storage{client: minioClient}, nil
}

func (s *S3Storage) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("error checking bucket %s: %w", bucket, err)
	}
	if !exists {
		err = s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("error creating bucket %s: %w", bucket, err)
		}
	}
	return nil
}

func (s *S3Storage) Upload(
	ctx context.Context,
	bucket string,
	key string,
	file io.Reader,
	size int64,
	contentType string,
) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	uploadInfo, err := s.client.PutObject(ctx, bucket, key, file, size, opts)
	if err != nil {
		return "", fmt.Errorf("failed to upload object %s to bucket %s: %w", key, bucket, err)
	}

	return uploadInfo.Key, nil
}

func (s *S3Storage) GetDownloadURL(ctx context.Context, bucket string, key string) (string, error) {
	// Set presigned expiration for 2 hours
	reqParams := make(url.Values)
	presignedURL, err := s.client.PresignedGetObject(ctx, bucket, key, 2*time.Hour, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned download URL for %s: %w", key, err)
	}

	return presignedURL.String(), nil
}
