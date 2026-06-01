package main

import (
	"context"
	"log"
	"os"

	"github.com/superteam/control-plane/internal/app"
	"github.com/superteam/control-plane/internal/config"
)

func main() {
	Main()
}

func Main() {
	configPath, err := config.ParseConfigPath(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("control-plane listening on %s", cfg.HTTP.Addr)
	if err := app.Run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}
