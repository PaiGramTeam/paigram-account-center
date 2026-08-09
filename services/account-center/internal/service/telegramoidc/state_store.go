package telegramoidc

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/model"
)

const (
	purposeTelegramOIDC = "telegram_oidc"
	stateTTL            = 10 * time.Minute
)

// ErrInvalidState is returned by Consume when the state token cannot be
// redeemed (missing, expired, already consumed, OR client_ip/user_agent
// mismatch — V23 hardening). The error is opaque on purpose: handlers must
// not surface the underlying reason to users; the user-facing reason code
// is always `state_invalid`.
var ErrInvalidState = errors.New("telegramoidc: invalid state")

// IssueInput captures everything required to mint a fresh state row.
type IssueInput struct {
	CodeVerifier string // 43-char URL-safe base64 (PKCE), caller-supplied
	BotID        string // Telegram bot identifier (e.g. "paigrambot")
	RedirectTo   string // browser-side path to 302 to after callback success
	ClientIP     string // c.ClientIP() captured at issue time; checked at consume
	UserAgent    string // c.GetHeader("User-Agent") captured at issue; checked at consume
}

// ConsumeInput is the request side of Consume. Caller MUST pass the live
// request's client IP and user agent so we can enforce the V23 binding rule.
type ConsumeInput struct {
	State     string
	ClientIP  string
	UserAgent string
}

// StateRecord is the result of a successful Consume.
type StateRecord struct {
	CodeVerifier string
	BotID        string
	RedirectTo   string
	ClientIP     string
	UserAgent    string
}

// StateStore persists OIDC PKCE state in user_oauth_states.
type StateStore struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewStateStore(db *gorm.DB, logger *zap.Logger) *StateStore {
	return &StateStore{db: db, logger: logger}
}

// Issue inserts a fresh state row and returns the 32-char hex token to embed
// in the OIDC authorize URL. The caller owns the code_verifier; we persist
// it on the dedicated code_verifier column (NOT Nonce — Nonce stays empty
// for purpose='telegram_oidc').
func (s *StateStore) Issue(ctx context.Context, in IssueInput) (string, error) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("telegramoidc: read random: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	metaBytes, err := json.Marshal(map[string]string{
		"bot_id": in.BotID,
	})
	if err != nil {
		return "", fmt.Errorf("telegramoidc: marshal metadata: %w", err)
	}

	row := model.UserOAuthState{
		Provider:     "telegram",
		Purpose:      purposeTelegramOIDC,
		State:        state,
		CodeVerifier: in.CodeVerifier,
		RedirectTo:   in.RedirectTo,
		ClientIP:     in.ClientIP,
		UserAgent:    in.UserAgent,
		Metadata:     datatypes.JSON(metaBytes),
		ExpiresAt:    time.Now().Add(stateTTL),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", fmt.Errorf("telegramoidc: insert state: %w", err)
	}
	return state, nil
}

// Consume atomically marks the row consumed and returns the stored values.
// SELECT FOR UPDATE serializes concurrent callbacks for the same state.
// V23 hardening: client IP and User-Agent at consume time MUST match the
// values captured at issue time — otherwise the state is rejected as if it
// did not exist.
//
// This is a backward-compatibility wrapper that opens its own short
// transaction and delegates to ConsumeTx. Callers that need to bundle the
// state consume into a larger transaction (e.g. handler/telegramoidc's
// Callback per spec §6.3) MUST call ConsumeTx directly with an outer tx.
func (s *StateStore) Consume(ctx context.Context, in ConsumeInput) (*StateRecord, error) {
	var record *StateRecord
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		record, err = s.ConsumeTx(ctx, tx, in)
		return err
	})
	if txErr != nil {
		return nil, txErr
	}
	return record, nil
}

// ConsumeTx is the transaction-aware variant of Consume. It runs against
// the caller-supplied tx — it MUST NOT call tx.Transaction(...) itself,
// since the SELECT FOR UPDATE row lock is owned by the outer tx for the
// duration of the caller's logical unit of work.
//
// Spec §6.3 (atomicity): the callback handler wraps steps 9a (this
// Consume) through 9f (session.Issue) in a single GORM transaction so a
// downstream failure rolls back the consumed_at marker. The trade-off:
// the state row's UPDATE lock is held across the outer-tx span, which
// includes the external token-exchange + JWT-verify HTTP RTT. This is
// acceptable for OIDC's sub-second round trip.
func (s *StateStore) ConsumeTx(ctx context.Context, tx *gorm.DB, in ConsumeInput) (*StateRecord, error) {
	var row model.UserOAuthState
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("state = ? AND purpose = ?", in.State, purposeTelegramOIDC).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidState
		}
		return nil, err
	}
	if row.ConsumedAt.Valid {
		return nil, ErrInvalidState
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, ErrInvalidState
	}
	if row.ClientIP != in.ClientIP || row.UserAgent != in.UserAgent {
		return nil, ErrInvalidState
	}
	row.ConsumedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if err := tx.Save(&row).Error; err != nil {
		return nil, err
	}

	var meta map[string]string
	if err := json.Unmarshal(row.Metadata, &meta); err != nil {
		return nil, fmt.Errorf("telegramoidc: unmarshal metadata: %w", err)
	}
	return &StateRecord{
		CodeVerifier: row.CodeVerifier,
		BotID:        meta["bot_id"],
		RedirectTo:   row.RedirectTo,
		ClientIP:     row.ClientIP,
		UserAgent:    row.UserAgent,
	}, nil
}
