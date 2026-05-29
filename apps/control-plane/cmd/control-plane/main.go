package main

import (
	"context"
	"flag"
	"log"

	"github.com/superteam/control-plane/internal/app"
	"github.com/superteam/control-plane/internal/config"
)

func main() {
	Main()
}

func Main() {
	configPath := flag.String("config", "", "path to control-plane YAML config file")
	flag.Parse()

	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("control-plane listening on %s", cfg.HTTP.Addr)
	if err := app.Run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}
