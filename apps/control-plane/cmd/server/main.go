package main

import (
	"context"
	"flag"
	"log"

	"github.com/superteam/control-plane/internal/app"
	"github.com/superteam/control-plane/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to control-plane YAML config file")
	flag.Parse()

	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Starting server on %s", cfg.HTTP.Addr)
	if err := app.Run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}
