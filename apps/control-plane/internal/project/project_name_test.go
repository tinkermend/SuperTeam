package project

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateProjectDirectoryName(t *testing.T) {
	t.Parallel()
	ok := []string{"a", "demo", "Demo_1", "my-project", "pkg.v2", "A1"}
	for _, name := range ok {
		if err := ValidateProjectDirectoryName(name); err != nil {
			t.Fatalf("%q should be valid: %v", name, err)
		}
	}
	bad := []string{"", "  demo", "客户接入", ".", "..", "foo/bar", "-leading", "trailing-", ".hidden", "has space"}
	for _, name := range bad {
		err := ValidateProjectDirectoryName(name)
		if err == nil {
			t.Fatalf("%q should be invalid", name)
		}
		if !errors.Is(err, ErrInvalidProjectName) {
			t.Fatalf("%q: want ErrInvalidProjectName, got %v", name, err)
		}
	}
	long := strings.Repeat("a", projectDirectoryNameMaxBytes+1)
	if err := ValidateProjectDirectoryName(long); err == nil {
		t.Fatal("overlong name should be invalid")
	}
}

func TestDirectoryNameFromGitURL(t *testing.T) {
	t.Parallel()
	got, err := DirectoryNameFromGitURL("https://github.com/acme/customer-onboarding.git")
	if err != nil {
		t.Fatalf("https url: %v", err)
	}
	if got != "customer-onboarding" {
		t.Fatalf("got %q", got)
	}
	got, err = DirectoryNameFromGitURL("git@github.com:acme/SuperTeam.git")
	if err != nil {
		t.Fatalf("scp url: %v", err)
	}
	if got != "SuperTeam" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateDisplayProjectNameAllowsChinese(t *testing.T) {
	t.Parallel()
	if err := ValidateDisplayProjectName("客户接入试点"); err != nil {
		t.Fatalf("chinese display name should be valid: %v", err)
	}
}
