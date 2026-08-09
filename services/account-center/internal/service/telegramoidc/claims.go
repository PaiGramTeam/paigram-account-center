// Package telegramoidc implements the Telegram OpenID Connect client used by
// account-center to authenticate Telegram users and atomically link their
// chat ids to user-center accounts.
//
// Spec: docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md
package telegramoidc

// Claims is the verified subset of the Telegram id_token JWT we consume.
// Fields not listed here are silently ignored so future-incompatible
// claim additions do not break login.
type Claims struct {
	Sub               string `json:"sub"`
	ID                int64  `json:"id"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Picture           string `json:"picture,omitempty"`
	Iss               string `json:"iss"`
	Aud               string `json:"aud"`
	Iat               int64  `json:"iat"`
	Exp               int64  `json:"exp"`
}
