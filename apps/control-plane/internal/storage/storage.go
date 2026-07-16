package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/smithy-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	PostgresURL string
	RedisURL    string
	ObjectStore ObjectStoreConfig
}

type ObjectStoreConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
}

type Clients struct {
	Postgres    *pgxpool.Pool
	Redis       *redis.Client
	ObjectStore *S3ObjectStore
}

func NewClients(ctx context.Context, cfg Config) (*Clients, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.PostgresURL)
	if err != nil {
		return nil, err
	}
	postgres, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}

	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		postgres.Close()
		return nil, err
	}

	s3Client, err := NewS3Client(ctx, cfg.ObjectStore)
	if err != nil {
		postgres.Close()
		return nil, err
	}

	objectStore, err := NewS3ObjectStore(s3Client, cfg.ObjectStore.Bucket)
	if err != nil {
		postgres.Close()
		return nil, err
	}
	objectStore.SetPresigner(s3.NewPresignClient(s3Client))

	clients := &Clients{
		Postgres:    postgres,
		Redis:       redis.NewClient(redisOptions),
		ObjectStore: objectStore,
	}

	return clients, nil
}

func (c *Clients) Close() {
	if c == nil {
		return
	}
	if c.Postgres != nil {
		c.Postgres.Close()
	}
	if c.Redis != nil {
		_ = c.Redis.Close()
	}
}

func (c Config) validate() error {
	if strings.TrimSpace(c.PostgresURL) == "" {
		return errors.New("postgres URL is required")
	}
	if strings.TrimSpace(c.RedisURL) == "" {
		return errors.New("redis URL is required")
	}
	if strings.TrimSpace(c.ObjectStore.Bucket) == "" {
		return errors.New("object store bucket is required")
	}
	return nil
}

func NewS3Client(ctx context.Context, cfg ObjectStoreConfig) (*s3.Client, error) {
	awsCfg, err := loadS3AWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
	}), nil
}

func loadS3AWSConfig(ctx context.Context, cfg ObjectStoreConfig) (aws.Config, error) {
	endpointResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               cfg.Endpoint,
			SigningRegion:     cfg.Region,
			HostnameImmutable: false,
		}, nil
	})

	return awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
		awsconfig.WithEndpointResolverWithOptions(endpointResolver),
	)
}

type S3API interface {
	PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, input *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3Presigner is the subset of *s3.PresignClient the object store needs; kept
// as an interface so tests can fake URL signing without AWS machinery.
type S3Presigner interface {
	PresignPutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
	PresignGetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

type S3ObjectStore struct {
	client    S3API
	presigner S3Presigner
	bucket    string
}

// ObjectStat is the existence/size answer used by evidence materialization to
// confirm a runtime-uploaded object really landed before writing DB rows.
type ObjectStat struct {
	Exists      bool
	SizeBytes   int64
	ContentType string
}

type PutObjectOptions struct {
	ContentType string
	Metadata    map[string]string
}

type ObjectRef struct {
	Bucket string
	Key    string
	URI    string
}

type Object struct {
	Body        io.ReadCloser
	ContentType string
	Metadata    map[string]string
}

func NewS3ObjectStore(client S3API, bucket string) (*S3ObjectStore, error) {
	if client == nil {
		return nil, errors.New("s3 client is required")
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, errors.New("object store bucket is required")
	}

	return &S3ObjectStore{
		client: client,
		bucket: bucket,
	}, nil
}

func (s *S3ObjectStore) PutObject(ctx context.Context, key string, body io.Reader, options PutObjectOptions) (ObjectRef, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return ObjectRef{}, errors.New("object key is required")
	}
	if body == nil {
		return ObjectRef{}, errors.New("object body is required")
	}

	input := &s3.PutObjectInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		Body:     body,
		Metadata: options.Metadata,
	}
	if strings.TrimSpace(options.ContentType) != "" {
		input.ContentType = aws.String(options.ContentType)
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return ObjectRef{}, fmt.Errorf("put object %q: %w", key, err)
	}

	return ObjectRef{
		Bucket: s.bucket,
		Key:    key,
		URI:    "s3://" + s.bucket + "/" + key,
	}, nil
}

func (s *S3ObjectStore) GetObject(ctx context.Context, key string) (*Object, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("object key is required")
	}

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}

	return &Object{
		Body:        output.Body,
		ContentType: aws.ToString(output.ContentType),
		Metadata:    output.Metadata,
	}, nil
}

func (s *S3ObjectStore) Exists(ctx context.Context, key string) (bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, errors.New("object key is required")
	}

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return false, nil
		}
	}

	return false, fmt.Errorf("head object %q: %w", key, err)
}

// SetPresigner attaches URL-presigning capability; without it Presign* fail.
func (s *S3ObjectStore) SetPresigner(presigner S3Presigner) {
	s.presigner = presigner
}

// StatObject returns existence and size without fetching the body. A missing
// object is (Exists=false, nil error); other failures are errors.
func (s *S3ObjectStore) StatObject(ctx context.Context, key string) (ObjectStat, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return ObjectStat{}, errors.New("object key is required")
	}

	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "NotFound", "NoSuchKey", "404":
				return ObjectStat{}, nil
			}
		}
		return ObjectStat{}, fmt.Errorf("head object %q: %w", key, err)
	}

	stat := ObjectStat{Exists: true, ContentType: aws.ToString(output.ContentType)}
	if output.ContentLength != nil {
		stat.SizeBytes = *output.ContentLength
	}
	return stat, nil
}

// PresignPut signs a direct-upload URL for key. Callers own key derivation and
// tenant scoping; this layer only signs within the configured bucket.
func (s *S3ObjectStore) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("object key is required")
	}
	if s.presigner == nil {
		return "", errors.New("object store presigner is not configured")
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(contentType)
	}
	request, err := s.presigner.PresignPutObject(ctx, input, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("presign put %q: %w", key, err)
	}
	return request.URL, nil
}

// PresignGet signs a direct-download URL for key.
func (s *S3ObjectStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("object key is required")
	}
	if s.presigner == nil {
		return "", errors.New("object store presigner is not configured")
	}

	request, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("presign get %q: %w", key, err)
	}
	return request.URL, nil
}

func (s *S3ObjectStore) DeleteObject(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("object key is required")
	}

	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}

	return nil
}
