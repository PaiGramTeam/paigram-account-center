//go:build integration

package e2ereal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewFixtureIdentityUsesNormalizedUniqueCredentials(t *testing.T) {
	first, err := newFixtureIdentity("browser.admin")
	if err != nil {
		t.Fatalf("newFixtureIdentity() error = %v", err)
	}
	second, err := newFixtureIdentity("browser.admin")
	if err != nil {
		t.Fatalf("newFixtureIdentity() second error = %v", err)
	}
	if first == second {
		t.Fatal("newFixtureIdentity() returned duplicate credentials")
	}
	if first.email != strings.ToLower(first.email) {
		t.Fatalf("email = %q, want lowercase", first.email)
	}
	if len(first.password) < 32 || !strings.Contains(first.password, "-aA1!") {
		t.Fatal("fixture password does not meet the expected strength profile")
	}
}

func TestWriteStateRemovesTemporaryFileWhenPublishFails(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	err := writeState(statePath, State{AdminPassword: "temporary-secret"})
	if err == nil {
		t.Fatal("writeState() error = nil, want publish failure")
	}
	if _, statErr := os.Stat(statePath + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("temporary state still exists: %v", statErr)
	}
}

func TestWritePlatformConfigIncludesPrivateMetricsListener(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.yaml")
	if err := writePlatformConfig(path, "postgres://integration", "redis:6379", "http://upstream", fixtureMaterial{}); err != nil {
		t.Fatalf("writePlatformConfig() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(body), "metrics:\n  addr: \"127.0.0.1:0\"") {
		t.Fatal("platform config does not define the private metrics listener")
	}
}
