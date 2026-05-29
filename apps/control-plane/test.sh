#!/bin/bash
# Test script for control-plane storage layer
# This script sets up the correct Docker/Podman environment for testcontainers

set -e

# Detect Podman socket location
PODMAN_SOCKET=$(ls /var/folders/*/T/podman/podman-machine-default-api.sock 2>/dev/null | head -1)

if [ -n "$PODMAN_SOCKET" ]; then
    echo "Using Podman socket: $PODMAN_SOCKET"
    export DOCKER_HOST="unix://$PODMAN_SOCKET"
    export TESTCONTAINERS_RYUK_DISABLED=true
fi

# Run tests
go test ./internal/storage/queries -v -timeout 5m "$@"
