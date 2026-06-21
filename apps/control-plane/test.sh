#!/bin/bash
# Test script for control-plane storage layer
# This script runs sqlc query integration tests against an explicit test environment.

set -e

if [ -z "${TEST_DATABASE_URL:-}" ] || [ -z "${TEST_REDIS_URL:-}" ]; then
    echo "Skipping storage query integration tests."
    echo "Set TEST_DATABASE_URL and TEST_REDIS_URL to run against a remote or dedicated test environment."
    echo "Do not point this test at the application DATABASE_URL; it truncates core business tables."
    exit 0
fi

# Run tests
go test ./internal/storage/queries -v -timeout 5m "$@"
