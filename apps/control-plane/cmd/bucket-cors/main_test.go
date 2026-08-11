package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestRuleCoversOrigins(t *testing.T) {
	rules := []types.CORSRule{{
		AllowedOrigins: []string{"http://127.0.0.1:3100", "http://localhost:3100"},
		AllowedMethods: []string{"GET", "HEAD"},
	}}
	if !ruleCoversOrigins(rules, []string{"http://127.0.0.1:3100"}) {
		t.Fatal("expected local origin covered")
	}
	if ruleCoversOrigins(rules, []string{"https://console.example.com"}) {
		t.Fatal("expected foreign origin not covered")
	}
	if !ruleCoversOrigins([]types.CORSRule{{AllowedOrigins: []string{"*"}}}, []string{"https://x.example"}) {
		t.Fatal("wildcard should cover any origin")
	}
}

func TestSplitCSVDedup(t *testing.T) {
	got := splitCSV(" http://a ,http://b,http://a, ")
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
}
