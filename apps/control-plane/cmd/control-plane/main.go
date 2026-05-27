package main

import (
	"context"
	"log"
	"net/http"

	"github.com/superteam/control-plane/internal/api"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/storage"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	stores, err := storage.NewClients(context.Background(), storage.Config{
		PostgresURL: cfg.Postgres.URL,
		RedisURL:    cfg.Redis.URL,
		ObjectStore: storage.ObjectStoreConfig{
			Endpoint:        cfg.ObjectStore.Endpoint,
			Region:          cfg.ObjectStore.Region,
			Bucket:          cfg.ObjectStore.Bucket,
			AccessKeyID:     cfg.ObjectStore.AccessKeyID,
			SecretAccessKey: cfg.ObjectStore.SecretAccessKey,
			ForcePathStyle:  cfg.ObjectStore.ForcePathStyle,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer stores.Close()

	log.Printf("control-plane listening on %s", cfg.HTTP.Addr)
	if err := http.ListenAndServe(cfg.HTTP.Addr, api.NewRouter()); err != nil {
		log.Fatal(err)
	}
}
