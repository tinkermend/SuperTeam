package main

import (
	"context"
	"flag"
	"log"

	"github.com/superteam/control-plane/internal/api"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/storage"
)

func main() {
	configPath := flag.String("config", "", "path to control-plane YAML config file")
	flag.Parse()

	cfg, err := config.LoadFromFile(*configPath)
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

	server := api.NewServer()

	log.Printf("Starting server on %s", cfg.HTTP.Addr)
	if err := server.Start(cfg.HTTP.Addr); err != nil {
		log.Fatal(err)
	}
}
