#!/bin/bash
# Test script for control-plane storage layer
# This script runs sqlc query integration tests against an explicit test environment.

set -e

if [ "${ALLOW_DATABASE_URL_FOR_QUERY_TESTS:-}" = "1" ] || [ "${ALLOW_DATABASE_URL_FOR_QUERY_TESTS:-}" = "true" ]; then
    : "${TEST_DATABASE_URL:=${DATABASE_URL:-}}"
    : "${TEST_REDIS_URL:=${REDIS_URL:-}}"
    export TEST_DATABASE_URL TEST_REDIS_URL
fi

if [ -z "${TEST_DATABASE_URL:-}" ] || [ -z "${TEST_REDIS_URL:-}" ]; then
    echo "Skipping storage query integration tests."
    echo "Set TEST_DATABASE_URL and TEST_REDIS_URL to run against a remote or dedicated test environment."
    echo "Alternatively set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL after confirming the database can be migrated and cleaned."
    exit 0
fi

# Run tests
go test ./internal/storage/queries -v -timeout 5m "$@"
