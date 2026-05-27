package main

import (
	"log"
	"net/http"
	"os"

	"github.com/superteam/control-plane/internal/api"
)

func main() {
	addr := os.Getenv("CONTROL_PLANE_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("control-plane listening on %s", addr)
	if err := http.ListenAndServe(addr, api.NewRouter()); err != nil {
		log.Fatal(err)
	}
}

