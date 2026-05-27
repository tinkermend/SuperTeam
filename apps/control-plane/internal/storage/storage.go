package storage

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	ObjectStore *s3.Client
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

	clients := &Clients{
		Postgres: postgres,
		Redis:    redis.NewClient(redisOptions),
		ObjectStore: s3.New(s3.Options{
			BaseEndpoint: aws.String(cfg.ObjectStore.Endpoint),
			Region:       cfg.ObjectStore.Region,
			Credentials: credentials.NewStaticCredentialsProvider(
				cfg.ObjectStore.AccessKeyID,
				cfg.ObjectStore.SecretAccessKey,
				"",
			),
			UsePathStyle: cfg.ObjectStore.ForcePathStyle,
		}),
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
