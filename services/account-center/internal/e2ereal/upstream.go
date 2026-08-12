//go:build integration

package e2ereal

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

func startUpstream() (*http.Server, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	var issued atomic.Uint64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/credentials:discover":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"account_id": "10001", "region": "cn_gf01", "profiles": e2eProfiles(),
			})
		case "/v1/credentials:refresh":
			var request struct {
				Credential string `json:"credential_bundle_json"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"credential_bundle_json": request.Credential, "account_id": "10001", "region": "cn_gf01",
				"profiles": e2eProfiles(), "expires_at": time.Now().UTC().Add(time.Hour),
			})
		case "/v1/authkeys:issue":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authkey": fmt.Sprintf("e2e-authkey-%d", issued.Add(1)), "expires_in_seconds": 300,
			})
		case "/v1/authkeys:revoke":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return server, "http://" + listener.Addr().String(), nil
}

func e2eProfiles() []map[string]any {
	return []map[string]any{
		{"GameBiz": "hk4e_cn", "Region": "cn_gf01", "PlayerID": "1008611", "Nickname": "Traveler", "Level": 60},
		{"GameBiz": "hk4e_cn", "Region": "cn_gf01", "PlayerID": "1008612", "Nickname": "Aether", "Level": 55},
	}
}
