//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

type mihomoProtocolSimulator struct {
	mu                      sync.Mutex
	issued                  int
	revoked                 []string
	revokeFailuresRemaining int
	server                  *httptest.Server
}

func newMihomoProtocolSimulator(t *testing.T) (*platformmihomo.HTTPClient, *mihomoProtocolSimulator) {
	t.Helper()
	simulator := &mihomoProtocolSimulator{}
	simulator.server = httptest.NewServer(http.HandlerFunc(simulator.serveHTTP))
	t.Cleanup(simulator.server.Close)
	client, err := platformmihomo.NewHTTPClient(platformmihomo.HTTPClientConfig{
		BaseURL: simulator.server.URL, Timeout: time.Second, AllowInsecureHTTP: true,
	})
	require.NoError(t, err)
	return client, simulator
}

func (s *mihomoProtocolSimulator) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/v1/credentials:discover":
		s.writeDiscovery(w)
	case "/v1/credentials:refresh":
		s.writeRefresh(w, r)
	case "/v1/authkeys:issue":
		s.writeAuthKey(w, r)
	case "/v1/authkeys:revoke":
		s.revokeAuthKey(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *mihomoProtocolSimulator) writeDiscovery(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"account_id": "10001", "region": "cn_gf01", "profiles": simulatorProfiles(),
	})
}

func (s *mihomoProtocolSimulator) writeRefresh(w http.ResponseWriter, r *http.Request) {
	var request struct {
		CredentialBundleJSON string `json:"credential_bundle_json"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"credential_bundle_json": request.CredentialBundleJSON,
		"account_id":             "10001",
		"region":                 "cn_gf01",
		"profiles":               simulatorProfiles(),
		"expires_at":             time.Now().UTC().Add(time.Hour),
	})
}

func (s *mihomoProtocolSimulator) writeAuthKey(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TTLSeconds int64 `json:"ttl_seconds"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || request.TTLSeconds != 300 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.issued++
	authKey := fmt.Sprintf("simulator-authkey-%d", s.issued)
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"authkey": authKey, "expires_in_seconds": request.TTLSeconds})
}

func (s *mihomoProtocolSimulator) revokeAuthKey(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AuthKey string `json:"authkey"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || request.AuthKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.revokeFailuresRemaining > 0 {
		s.revokeFailuresRemaining--
		s.mu.Unlock()
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	s.revoked = append(s.revoked, request.AuthKey)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *mihomoProtocolSimulator) failNextRevocations(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokeFailuresRemaining = count
}

func (s *mihomoProtocolSimulator) revokedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.revoked)
}

func simulatorProfiles() []platformmihomo.DiscoveredProfile {
	return []platformmihomo.DiscoveredProfile{
		{GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "1008611", Nickname: "Traveler", Level: 60},
		{GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "1008612", Nickname: "Aether", Level: 55},
	}
}
