package storage

import "testing"

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
