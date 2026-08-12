package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Consumer identifiers. Under Path D Option D the consumer name IS the
// OAuth client_id of the caller (Path D §10 Q5 + bot_access_service.go).
// We keep these two well-known constants for back-compatibility with the
// existing platformbinding tests and the legacy /me/platform-accounts/
// :bindingId/consumer-grants/:consumer admin/owner routes; they are no
// longer derived from a model-level map.
const (
	ConsumerPaiGramBot = "paigram-bot"
	ConsumerPamgram    = "pamgram"
)

// SupportedConsumers contains built-in principals accepted before an OAuth
// credential is provisioned. Registered active service credentials are also
// accepted dynamically.
var SupportedConsumers = []string{ConsumerPaiGramBot, ConsumerPamgram}

type CreateBindingInput struct {
	OwnerUserID        uint64
	Platform           string
	ExternalAccountKey sql.NullString
	PlatformServiceKey string
	DisplayName        string
}

type CreateAndBindInput struct {
	OwnerUserID       uint64
	Platform          string
	DisplayName       string
	ActorType         string
	ActorID           string
	CredentialPayload json.RawMessage
}

type UpsertGrantInput struct {
	BindingID uint64
	Consumer  string
	Actions   []string
	GrantedBy sql.NullInt64
	GrantedAt time.Time
}

type RevokeGrantInput struct {
	Context     context.Context
	BindingID   uint64
	Consumer    string
	RevokedAt   time.Time
	ActorUserID sql.NullInt64
}

type ProfileProjectionInput struct {
	PlatformProfileKey string
	GameBiz            string
	Region             string
	PlayerUID          string
	Nickname           string
	Level              sql.NullInt64
	IsPrimary          bool
	SourceUpdatedAt    sql.NullTime
}

type SyncProfilesInput struct {
	BindingID uint64
	Profiles  []ProfileProjectionInput
	SyncedAt  time.Time
}

type PutCredentialInput struct {
	OwnerUserID        uint64
	BindingID          uint64
	ActorType          string
	ActorID            string
	RequestedByAdminID uint64
	CredentialPayload  json.RawMessage
}

type RuntimeSummary struct {
	PlatformAccountID string           `json:"platform_account_id"`
	Status            string           `json:"status"`
	LastValidatedAt   any              `json:"last_validated_at"`
	LastRefreshedAt   any              `json:"last_refreshed_at"`
	Devices           []map[string]any `json:"devices"`
	Profiles          []map[string]any `json:"profiles"`
}
