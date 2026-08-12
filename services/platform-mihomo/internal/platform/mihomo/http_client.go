package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxUpstreamResponseBytes = 1 << 20

type HTTPClientConfig struct {
	BaseURL           string
	Timeout           time.Duration
	BearerTokenFile   string
	AllowInsecureHTTP bool
	HTTPClient        *http.Client
}

type HTTPClient struct {
	baseURL     *url.URL
	httpClient  *http.Client
	bearerToken string
}

type discoverRequest struct {
	CredentialBundleJSON string `json:"credential_bundle_json"`
	RegionHint           string `json:"region_hint,omitempty"`
}

type discoverResponse struct {
	AccountID string              `json:"account_id"`
	Region    string              `json:"region"`
	Profiles  []DiscoveredProfile `json:"profiles"`
}

type refreshResponse struct {
	CredentialBundleJSON string              `json:"credential_bundle_json"`
	AccountID            string              `json:"account_id"`
	Region               string              `json:"region"`
	Profiles             []DiscoveredProfile `json:"profiles"`
	ExpiresAt            time.Time           `json:"expires_at"`
}

type authKeyRequest struct {
	CredentialBundleJSON string `json:"credential_bundle_json"`
	PlayerID             string `json:"player_id"`
}

type authKeyResponse struct {
	AuthKey          string `json:"authkey"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

func NewHTTPClient(cfg HTTPClientConfig) (*HTTPClient, error) {
	baseURL, err := parseBaseURL(cfg.BaseURL, cfg.AllowInsecureHTTP)
	if err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("mihomo upstream timeout must be greater than zero")
	}
	bearerToken := ""
	if strings.TrimSpace(cfg.BearerTokenFile) != "" {
		raw, err := os.ReadFile(cfg.BearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("read mihomo upstream bearer token file: %w", err)
		}
		bearerToken = strings.TrimSpace(string(raw))
		if bearerToken == "" {
			return nil, errors.New("mihomo upstream bearer token file is empty")
		}
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &HTTPClient{baseURL: baseURL, httpClient: httpClient, bearerToken: bearerToken}, nil
}

func (c *HTTPClient) ValidateAndDiscover(ctx context.Context, cookieBundleJSON string, regionHint string) (string, string, []DiscoveredProfile, error) {
	var response discoverResponse
	if err := c.postJSON(ctx, "/v1/credentials:discover", discoverRequest{
		CredentialBundleJSON: cookieBundleJSON,
		RegionHint:           regionHint,
	}, &response); err != nil {
		return "", "", nil, err
	}
	if strings.TrimSpace(response.AccountID) == "" || strings.TrimSpace(response.Region) == "" {
		return "", "", nil, &UpstreamError{Kind: ErrorInvalidResponse}
	}
	return response.AccountID, response.Region, response.Profiles, nil
}

func (c *HTTPClient) RefreshCredential(ctx context.Context, cookieBundleJSON string, regionHint string) (RefreshResult, error) {
	var response refreshResponse
	if err := c.postJSON(ctx, "/v1/credentials:refresh", discoverRequest{
		CredentialBundleJSON: cookieBundleJSON,
		RegionHint:           regionHint,
	}, &response); err != nil {
		return RefreshResult{}, err
	}
	result := RefreshResult{
		CredentialBundleJSON: response.CredentialBundleJSON,
		AccountID:            response.AccountID,
		Region:               response.Region,
		Profiles:             response.Profiles,
		ExpiresAt:            response.ExpiresAt.UTC(),
	}
	if err := ValidateRefreshResult(result, time.Now().UTC()); err != nil {
		return RefreshResult{}, err
	}
	return result, nil
}

func (c *HTTPClient) IssueAuthKey(ctx context.Context, cookieBundleJSON string, playerID string) (string, int64, error) {
	var response authKeyResponse
	if err := c.postJSON(ctx, "/v1/authkeys:issue", authKeyRequest{
		CredentialBundleJSON: cookieBundleJSON,
		PlayerID:             playerID,
	}, &response); err != nil {
		return "", 0, err
	}
	if strings.TrimSpace(response.AuthKey) == "" || response.ExpiresInSeconds <= 0 {
		return "", 0, &UpstreamError{Kind: ErrorInvalidResponse}
	}
	return response.AuthKey, response.ExpiresInSeconds, nil
}

func (c *HTTPClient) postJSON(ctx context.Context, path string, input, output any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode mihomo upstream request: %w", err)
	}
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: strings.TrimSuffix(c.baseURL.Path, "/") + path})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create mihomo upstream request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &UpstreamError{Kind: ErrorUnavailable}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return classifyHTTPError(resp)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxUpstreamResponseBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return &UpstreamError{Kind: ErrorInvalidResponse, StatusCode: resp.StatusCode}
	}
	return nil
}

func parseBaseURL(raw string, allowInsecureHTTP bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return nil, errors.New("mihomo upstream base_url must be an absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("mihomo upstream base_url must not contain credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && !(allowInsecureHTTP && parsed.Scheme == "http") {
		return nil, errors.New("mihomo upstream base_url must use https")
	}
	return parsed, nil
}

func classifyHTTPError(resp *http.Response) error {
	kind := ErrorUnavailable
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = ErrorInvalidCredential
	case http.StatusGone:
		kind = ErrorExpiredCredential
	case http.StatusConflict, http.StatusPreconditionRequired:
		kind = ErrorChallengeRequired
	case http.StatusTooManyRequests:
		kind = ErrorRateLimited
	}
	return &UpstreamError{
		Kind:       kind,
		StatusCode: resp.StatusCode,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}
}

func parseRetryAfter(raw string) time.Duration {
	seconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

var _ Client = (*HTTPClient)(nil)
var _ CredentialRefresher = (*HTTPClient)(nil)
