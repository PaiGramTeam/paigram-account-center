//go:build integration

package e2ereal

import (
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
