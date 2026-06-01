package config

import "testing"

func TestParseConfigPathAcceptsPackageManagerSeparator(t *testing.T) {
	path, err := ParseConfigPath([]string{"--", "--config", "apps/control-plane/config/config.yaml"})
	if err != nil {
		t.Fatalf("expected config flag to parse: %v", err)
	}

	if path != "apps/control-plane/config/config.yaml" {
		t.Fatalf("expected config path after separator, got %q", path)
	}
}

func TestParseConfigPathAcceptsDirectGoFlags(t *testing.T) {
	path, err := ParseConfigPath([]string{"--config", "apps/control-plane/config/config.yaml"})
	if err != nil {
		t.Fatalf("expected config flag to parse: %v", err)
	}

	if path != "apps/control-plane/config/config.yaml" {
		t.Fatalf("expected direct config path, got %q", path)
	}
}

func TestParseConfigPathAllowsPackageManagerArgsToOverrideDefaultScriptArgs(t *testing.T) {
	path, err := ParseConfigPath([]string{
		"--config",
		"apps/control-plane/config/config.yaml",
		"--",
		"--config",
		"/tmp/override.yaml",
	})
	if err != nil {
		t.Fatalf("expected config flag to parse: %v", err)
	}

	if path != "/tmp/override.yaml" {
		t.Fatalf("expected appended config path to override default script arg, got %q", path)
	}
}
