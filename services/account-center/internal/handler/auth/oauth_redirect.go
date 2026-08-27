package auth

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"paigram/internal/config"
)

var errOAuthRedirectURIInvalid = errors.New("oauth redirect URI is invalid")

func resolveOAuthRedirectURI(
	requested string,
	providerConfig config.OAuthProviderConfig,
	defaultRedirectURI string,
) (string, error) {
	candidate := strings.TrimSpace(requested)
	if candidate == "" {
		candidate = strings.TrimSpace(providerConfig.RedirectURL)
	}
	if candidate == "" {
		candidate = strings.TrimSpace(defaultRedirectURI)
	}
	if err := validateOAuthRedirectURI(candidate); err != nil {
		return "", err
	}

	allowed := append([]string{providerConfig.RedirectURL, defaultRedirectURI}, providerConfig.RedirectURLs...)
	for _, configured := range allowed {
		if candidate == strings.TrimSpace(configured) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: URI is not in the provider allowlist", errOAuthRedirectURIInvalid)
}

func validateOAuthRedirectURI(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%w: expected an absolute callback URI", errOAuthRedirectURIInvalid)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme != "http" || !isLoopbackOAuthHost(parsed.Hostname()) {
		return fmt.Errorf("%w: HTTPS is required outside loopback development", errOAuthRedirectURIInvalid)
	}
	return nil
}

func isLoopbackOAuthHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
