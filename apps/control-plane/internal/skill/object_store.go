package skill

import (
	"context"
	"io"
	"time"

	"github.com/superteam/control-plane/internal/storage"
)

type s3ObjectStore struct {
	inner *storage.S3ObjectStore
}

func WrapS3ObjectStore(inner *storage.S3ObjectStore) ObjectStore {
	if inner == nil {
		return nil
	}
	return s3ObjectStore{inner: inner}
}

func (s s3ObjectStore) PutObject(ctx context.Context, key string, body io.Reader, options storage.PutObjectOptions) (storage.ObjectRef, error) {
	return s.inner.PutObject(ctx, key, body, options)
}

func (s s3ObjectStore) DeleteObject(ctx context.Context, key string) error {
	return s.inner.DeleteObject(ctx, key)
}

func (s s3ObjectStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.inner.PresignGet(ctx, key, ttl)
}

func (s s3ObjectStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.inner.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	return obj.Body, nil
}
