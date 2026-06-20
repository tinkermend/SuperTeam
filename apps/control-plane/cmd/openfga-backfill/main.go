package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type backfillConfig struct {
	DatabaseURL string
	OpenFGA     authz.OpenFGAClientConfig
}

func main() {
	if err := run(context.Background(), loadConfigFromEnv()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg backfillConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := authz.NewPgRepository(queries.New(pool))
	syncer := authz.NewOpenFGATupleSyncer(authz.NewOpenFGAHTTPClient(cfg.OpenFGA), repo)
	if err := syncer.Backfill(ctx); err != nil {
		return err
	}
	log.Printf("openfga backfill completed store_id=%s model_id=%s", cfg.OpenFGA.StoreID, cfg.OpenFGA.ModelID)
	return nil
}

func loadConfigFromEnv() backfillConfig {
	return backfillConfig{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		OpenFGA: authz.OpenFGAClientConfig{
			APIURL:   os.Getenv("OPENFGA_API_URL"),
			StoreID:  os.Getenv("OPENFGA_STORE_ID"),
			ModelID:  os.Getenv("OPENFGA_MODEL_ID"),
			APIToken: os.Getenv("OPENFGA_API_TOKEN"),
		},
	}
}

func validateConfig(cfg backfillConfig) error {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	if strings.TrimSpace(cfg.OpenFGA.APIURL) == "" {
		return errors.New("OPENFGA_API_URL is required")
	}
	if strings.TrimSpace(cfg.OpenFGA.StoreID) == "" {
		return errors.New("OPENFGA_STORE_ID is required")
	}
	if strings.TrimSpace(cfg.OpenFGA.ModelID) == "" {
		return errors.New("OPENFGA_MODEL_ID is required")
	}
	return nil
}
