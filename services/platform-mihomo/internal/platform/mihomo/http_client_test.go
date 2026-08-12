package mihomo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPClientDiscoversProfilesWithoutLeakingCredentialIntoURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/credentials:discover", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		var request discoverRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.JSONEq(t, `{"cookie_token":"secret"}`, request.CredentialBundleJSON)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"account_id":"10001","region":"cn_gf01","profiles":[{"GameBiz":"hk4e_cn","Region":"cn_gf01","PlayerID":"1008611","Nickname":"Traveler","Level":60}]}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL, Timeout: time.Second, AllowInsecureHTTP: true})
	require.NoError(t, err)
	accountID, region, profiles, err := client.ValidateAndDiscover(context.Background(), `{"cookie_token":"secret"}`, "")
	require.NoError(t, err)
	require.Equal(t, "10001", accountID)
	require.Equal(t, "cn_gf01", region)
	require.Len(t, profiles, 1)
	require.Equal(t, "1008611", profiles[0].PlayerID)
}

func TestHTTPClientMapsRateLimitWithoutReturningResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`credential secret should never escape`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL, Timeout: time.Second, AllowInsecureHTTP: true})
	require.NoError(t, err)
	_, _, err = client.IssueAuthKey(context.Background(), `{}`, "1008611")
	require.Error(t, err)
	require.True(t, IsErrorKind(err, ErrorRateLimited))
	require.NotContains(t, err.Error(), "credential secret")
	var upstreamErr *UpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, 7*time.Second, upstreamErr.RetryAfter)
}

func TestHTTPClientRequiresHTTPSByDefault(t *testing.T) {
	_, err := NewHTTPClient(HTTPClientConfig{BaseURL: "http://upstream.example", Timeout: time.Second})
	require.EqualError(t, err, "mihomo upstream base_url must use https")
}
