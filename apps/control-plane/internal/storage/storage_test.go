package storage

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/stretchr/testify/assert"
)

func TestNewClientsAcceptStorageConfigWithoutNetworkDial(t *testing.T) {
	cfg := Config{
		PostgresURL: "postgres://superteam:secret@127.0.0.1:5432/superteam?sslmode=disable",
		RedisURL:    "redis://:secret@127.0.0.1:6379/0",
		ObjectStore: ObjectStoreConfig{
			Endpoint:        "http://127.0.0.1:9000",
			Region:          "us-east-1",
			Bucket:          "superteam-artifacts",
			AccessKeyID:     "minio",
			SecretAccessKey: "minio-secret",
			ForcePathStyle:  true,
		},
	}

	clients, err := NewClients(t.Context(), cfg)
	if err != nil {
		t.Fatalf("expected clients to initialize: %v", err)
	}
	defer clients.Close()

	if clients.Postgres == nil {
		t.Fatal("expected postgres pool")
	}
	if clients.Redis == nil {
		t.Fatal("expected redis client")
	}
	if clients.ObjectStore == nil {
		t.Fatal("expected object store client")
	}
}

func TestNewClientsRejectsMissingConfig(t *testing.T) {
	_, err := NewClients(t.Context(), Config{})
	if err == nil {
		t.Fatal("expected missing config to fail")
	}
}

func TestLoadS3AWSConfigUsesStaticCredentialsAndCustomEndpoint(t *testing.T) {
	cfg := ObjectStoreConfig{
		Endpoint:        "https://tos-s3-cn-beijing.volces.com",
		Region:          "cn-beijing",
		Bucket:          "superteam-artifacts",
		AccessKeyID:     "volc-ak",
		SecretAccessKey: "volc-sk",
		ForcePathStyle:  false,
	}

	awsCfg, err := loadS3AWSConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("expected AWS config: %v", err)
	}

	if awsCfg.Region != "cn-beijing" {
		t.Fatalf("expected signing region cn-beijing, got %q", awsCfg.Region)
	}

	creds, err := awsCfg.Credentials.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("expected credentials: %v", err)
	}
	if creds.AccessKeyID != "volc-ak" || creds.SecretAccessKey != "volc-sk" {
		t.Fatalf("expected static credentials, got access key %q", creds.AccessKeyID)
	}

	endpoint, err := awsCfg.EndpointResolverWithOptions.ResolveEndpoint(s3.ServiceID, "ignored")
	if err != nil {
		t.Fatalf("expected endpoint: %v", err)
	}
	if endpoint.URL != cfg.Endpoint {
		t.Fatalf("expected endpoint %q, got %q", cfg.Endpoint, endpoint.URL)
	}
	if endpoint.SigningRegion != cfg.Region {
		t.Fatalf("expected signing region %q, got %q", cfg.Region, endpoint.SigningRegion)
	}
	if endpoint.HostnameImmutable {
		t.Fatal("expected mutable hostname so virtual-hosted bucket addressing remains available")
	}
}
