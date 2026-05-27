package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/superteam/runtime-agent/internal/daemon"
)

func main() {
	once := flag.Bool("once", false, "print a startup snapshot and exit")
	nodeID := flag.String("node-id", getenv("RUNTIME_NODE_ID", "local-dev-node"), "runtime node id")
	flag.Parse()

	runner, err := daemon.NewRunner(daemon.Config{NodeID: *nodeID})
	if err != nil {
		log.Fatal(err)
	}

	snapshot := runner.Snapshot()
	fmt.Printf("runtime-agent node=%s status=%s\n", snapshot.NodeID, snapshot.Status)

	if *once {
		return
	}

	select {}
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

