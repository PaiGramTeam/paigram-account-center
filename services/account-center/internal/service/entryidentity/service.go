package entryidentity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/model"
	"paigram/internal/service/botlink"
	"paigram/internal/service/platformbinding"
)

const (
	challengeEntropyBytes = 32
	defaultChallengeTTL   = 5 * time.Minute
	maxPendingChallenges  = 100
)

type Config struct {
	FrontendBaseURL  string
	ChallengeTTL     time.Duration
	Now              func() time.Time
	Random           io.Reader
	GrantInvalidator platformbinding.GrantInvalidator
}

type StartInput struct {
	Consumer         string
	BotID            string
	ExternalSubject  string
	ExternalUsername string
}

type ChallengeView struct {
	Issuer         string    `json:"issuer"`
	MaskedSubject  string    `json:"masked_subject"`
	BotID          string    `json:"bot_id"`
	BotDisplayName string    `json:"bot_display_name"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type StartResult struct {
	ChallengeView
	ApprovalURL string `json:"approval_url"`
}

type UnlinkResult struct {
	OperationID        string `json:"operation_id"`
	MinimumEntryEpoch  uint64 `json:"minimum_entry_epoch"`
	PropagationPending bool   `json:"propagation_pending"`
	State              string `json:"state"`
}

type Service struct {
	db              *gorm.DB
	logger          *zap.Logger
	frontendBaseURL string
	challengeTTL    time.Duration
	now             func() time.Time
	random          io.Reader
	linker          *botlink.Service
	grants          *platformbinding.GrantService
}

func NewService(db *gorm.DB, logger *zap.Logger, cfg Config) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	challengeTTL := cfg.ChallengeTTL
	if challengeTTL <= 0 {
		challengeTTL = defaultChallengeTTL
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	random := cfg.Random
	if random == nil {
		random = rand.Reader
	}
	return &Service{
		db: db, logger: logger, frontendBaseURL: strings.TrimSpace(cfg.FrontendBaseURL),
		challengeTTL: challengeTTL, now: now, random: random, linker: botlink.NewService(db, logger),
		grants: platformbinding.NewGrantService(db, cfg.GrantInvalidator),
	}
}

func (s *Service) Start(ctx context.Context, input StartInput) (*StartResult, error) {
	if s == nil || s.db == nil || strings.TrimSpace(input.Consumer) == "" || strings.TrimSpace(input.BotID) == "" ||
		strings.TrimSpace(input.ExternalSubject) == "" || utf8.RuneCountInString(input.ExternalSubject) > 191 ||
		utf8.RuneCountInString(input.ExternalUsername) > 255 {
		return nil, ErrInvalidInput
	}

	var bot model.Bot
	tokenBytes := make([]byte, challengeEntropyBytes)
	if _, err := io.ReadFull(s.random, tokenBytes); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	now := s.now().UTC()
	challenge := model.EntryIdentityLinkChallenge{
		ChallengeHash: hashChallengeBytes(tokenBytes), Consumer: input.Consumer, BotID: input.BotID,
		Issuer: bot.EntryIssuer, ExternalSubject: input.ExternalSubject,
		Status: model.EntryIdentityLinkChallengePending, ExpiresAt: now.Add(s.challengeTTL),
		CreatedAt: now, UpdatedAt: now,
	}
	if input.ExternalUsername != "" {
		challenge.ExternalUsername = sql.NullString{String: input.ExternalUsername, Valid: true}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var credential model.ServiceCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("client_id", "bot_id", "status").
			Where("client_id = ? AND bot_id = ? AND status = ?", input.Consumer, input.BotID, model.ServiceCredentialStatusActive).
			First(&credential).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Error("entry identity principal lookup failed", zap.Error(err))
			}
			return ErrPrincipalMismatch
		}
		if err := tx.Select("id", "entry_issuer", "display_name", "status").
			Where("id = ? AND status = ?", input.BotID, "ACTIVE").First(&bot).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Error("entry identity bot lookup failed", zap.Error(err))
				return err
			}
			return ErrNamespaceUnavailable
		}
		if strings.TrimSpace(bot.EntryIssuer) == "" {
			return ErrNamespaceUnavailable
		}
		challenge.Issuer = bot.EntryIssuer
		var pending int64
		if err := tx.Model(&model.EntryIdentityLinkChallenge{}).
			Where("consumer = ? AND status = ? AND expires_at > ?", input.Consumer, model.EntryIdentityLinkChallengePending, now).
			Count(&pending).Error; err != nil {
			return err
		}
		if pending >= maxPendingChallenges {
			return ErrChallengeCapacity
		}
		return tx.Create(&challenge).Error
	}); err != nil {
		return nil, err
	}
	approvalURL, err := buildApprovalURL(s.frontendBaseURL, token)
	if err != nil {
		_ = s.db.WithContext(ctx).Delete(&challenge).Error
		return nil, err
	}
	return &StartResult{
		ChallengeView: ChallengeView{
			Issuer: challenge.Issuer, MaskedSubject: maskSubject(challenge.ExternalSubject),
			BotID: bot.ID, BotDisplayName: bot.DisplayName, ExpiresAt: challenge.ExpiresAt,
		},
		ApprovalURL: approvalURL,
	}, nil
}

func (s *Service) Preview(ctx context.Context, token string) (*ChallengeView, error) {
	challenge, bot, err := s.loadPending(ctx, s.db, token, false)
	if err != nil {
		return nil, err
	}
	return &ChallengeView{
		Issuer: challenge.Issuer, MaskedSubject: maskSubject(challenge.ExternalSubject),
		BotID: challenge.BotID, BotDisplayName: bot.DisplayName, ExpiresAt: challenge.ExpiresAt,
	}, nil
}

func (s *Service) Approve(ctx context.Context, userID uint64, token, requestIP, requestUA string) (*model.BotIdentity, error) {
	if userID == 0 {
		return nil, ErrInvalidInput
	}
	var identity *model.BotIdentity
	var outcome error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, _, err := s.loadPending(ctx, tx, token, true)
		if err != nil {
			return err
		}
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&user, userID).Error; err != nil {
			return err
		}
		username := nullStringPointer(challenge.ExternalUsername)
		linked, err := s.linker.UpsertLinkTx(ctx, tx, botlink.UpsertLinkInput{
			BotID: challenge.BotID, Issuer: challenge.Issuer, UserID: userID,
			ExternalUserID: challenge.ExternalSubject, ExternalUsername: username,
			RequestIP: requestIP, RequestUA: requestUA,
		})
		if errors.Is(err, botlink.ErrTelegramAlreadyLinkedToOther) || errors.Is(err, botlink.ErrAlreadyLinked) {
			if updateErr := consumeChallenge(tx, challenge.ChallengeHash, model.EntryIdentityLinkChallengeConflict, userID, s.now().UTC()); updateErr != nil {
				return updateErr
			}
			outcome = ErrIdentityConflict
			return nil
		}
		if errors.Is(err, botlink.ErrUnlinkPending) {
			outcome = ErrUnlinkPending
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.alignEntryEpoch(tx, linked); err != nil {
			return err
		}
		if err := consumeChallenge(tx, challenge.ChallengeHash, model.EntryIdentityLinkChallengeApproved, userID, s.now().UTC()); err != nil {
			return err
		}
		identity = linked
		return nil
	})
	if err != nil {
		return nil, err
	}
	if outcome != nil {
		return nil, outcome
	}
	return identity, nil
}

func (s *Service) Cancel(ctx context.Context, userID uint64, token string) error {
	if userID == 0 {
		return ErrInvalidInput
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, _, err := s.loadPending(ctx, tx, token, true)
		if err != nil {
			return err
		}
		return consumeChallenge(tx, challenge.ChallengeHash, model.EntryIdentityLinkChallengeCancelled, userID, s.now().UTC())
	})
}

func (s *Service) loadPending(ctx context.Context, db *gorm.DB, token string, lock bool) (*model.EntryIdentityLinkChallenge, *model.Bot, error) {
	hash, err := challengeHash(token)
	if err != nil {
		return nil, nil, ErrChallengeNotFound
	}
	var challenge model.EntryIdentityLinkChallenge
	query := db.WithContext(ctx).Session(&gorm.Session{NewDB: true})
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Where("challenge_hash = ?", hash).First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrChallengeNotFound
		}
		return nil, nil, err
	}
	if challenge.Status != model.EntryIdentityLinkChallengePending {
		return nil, nil, ErrChallengeConsumed
	}
	now := s.now().UTC()
	if !challenge.ExpiresAt.After(now) {
		if err := db.WithContext(ctx).Session(&gorm.Session{NewDB: true}).Model(&challenge).Updates(map[string]any{
			"status": model.EntryIdentityLinkChallengeExpired, "consumed_at": now, "updated_at": now,
		}).Error; err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrChallengeExpired
	}
	var credential model.ServiceCredential
	credentialQuery := db.WithContext(ctx).Session(&gorm.Session{NewDB: true})
	if lock {
		credentialQuery = credentialQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := credentialQuery.Select("client_id", "bot_id", "status").
		Where("client_id = ? AND bot_id = ? AND status = ?", challenge.Consumer, challenge.BotID, model.ServiceCredentialStatusActive).
		First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrPrincipalMismatch
		}
		return nil, nil, err
	}
	var bot model.Bot
	botQuery := db.WithContext(ctx).Session(&gorm.Session{NewDB: true})
	if lock {
		botQuery = botQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := botQuery.Select("id", "entry_issuer", "display_name", "status").
		Where("id = ? AND status = ?", challenge.BotID, "ACTIVE").First(&bot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNamespaceUnavailable
		}
		return nil, nil, err
	}
	if strings.TrimSpace(bot.EntryIssuer) == "" || bot.EntryIssuer != challenge.Issuer {
		return nil, nil, ErrNamespaceUnavailable
	}
	return &challenge, &bot, nil
}

func (s *Service) alignEntryEpoch(tx *gorm.DB, identity *model.BotIdentity) error {
	if identity == nil {
		return ErrInvalidInput
	}
	var maximum uint64
	if err := tx.Unscoped().Model(&model.BotIdentity{}).
		Where("user_id = ?", identity.UserID).
		Select("COALESCE(MAX(entry_epoch), 1)").Scan(&maximum).Error; err != nil {
		return err
	}
	if maximum <= identity.EntryEpoch {
		return nil
	}
	if err := tx.Unscoped().Model(identity).Update("entry_epoch", maximum).Error; err != nil {
		return err
	}
	identity.EntryEpoch = maximum
	return nil
}

func consumeChallenge(tx *gorm.DB, hash string, status model.EntryIdentityLinkChallengeStatus, userID uint64, now time.Time) error {
	result := tx.Model(&model.EntryIdentityLinkChallenge{}).
		Where("challenge_hash = ? AND status = ?", hash, model.EntryIdentityLinkChallengePending).
		Updates(map[string]any{
			"status": status, "approved_user_id": userID, "consumed_at": now, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrChallengeConsumed
	}
	return nil
}

func buildApprovalURL(baseURL, token string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil || !base.IsAbs() || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return "", ErrInvalidInput
	}
	fragment := url.Values{"challenge": []string{token}}.Encode()
	return base.ResolveReference(&url.URL{Path: "/entry-identity-link", Fragment: fragment}).String(), nil
}

func challengeHash(token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != challengeEntropyBytes {
		return "", ErrChallengeNotFound
	}
	return hashChallengeBytes(raw), nil
}

func hashChallengeBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func maskSubject(subject string) string {
	runes := []rune(subject)
	if len(runes) == 0 {
		return ""
	}
	if len(runes) == 1 {
		return "*"
	}
	if len(runes) == 2 {
		return string(runes[0]) + "*"
	}
	visible := 1
	if len(runes) >= 5 {
		visible = 2
	}
	return string(runes[:visible]) + strings.Repeat("*", len(runes)-(visible*2)) + string(runes[len(runes)-visible:])
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
