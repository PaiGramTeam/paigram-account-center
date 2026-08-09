package credentials

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"paigram/internal/model"
)

// Service manages OAuth 2.0 client_credentials registry rows
// (model.ServiceCredential). All admin CRUD goes through here.
type Service struct {
	db *gorm.DB
}

// NewService constructs a credentials service backed by gorm.DB.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateInput carries the operator-supplied data for a new credential.
// ClientID is provided directly by the caller (Path D §2.1: human-readable
// strings like "telegram-service", "paigram-genshin", no "mi_" prefix).
type CreateInput struct {
	ClientID    string
	DisplayName string
	OwnerUserID uint64
	Description string
	Audiences   []string
	Scopes      []string
}

// CredentialView is the safe outward-facing projection of a credential
// row (no secret hash). Used by admin handlers.
type CredentialView struct {
	ClientID    string    `json:"client_id"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	OwnerUserID uint64    `json:"owner_user_id"`
	Description string    `json:"description"`
	Audiences   []string  `json:"audiences"`
	Scopes      []string  `json:"scopes"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateResult bundles a freshly-created credential row with its plaintext
// secret. The plaintext is returned to the operator exactly once at
// creation time; subsequent reads expose only the bcrypt hash.
type CreateResult struct {
	Credential   *model.ServiceCredential `json:"-"`
	View         *CredentialView          `json:"credential"`
	ClientID     string                   `json:"client_id"`
	ClientSecret string                   `json:"client_secret"`
}

// Create inserts a new credential row, generates + bcrypt-hashes the
// plaintext secret, and returns both the persisted row and the plaintext
// secret to the caller.
func (s *Service) Create(input CreateInput) (*CreateResult, error) {
	clientID := strings.TrimSpace(input.ClientID)
	if clientID == "" {
		return nil, ErrEmptyClientID
	}

	audiencesJSON, err := encodeStringList(input.Audiences)
	if err != nil {
		return nil, err
	}
	scopesJSON, err := encodeStringList(input.Scopes)
	if err != nil {
		return nil, err
	}

	plaintext, err := GenerateClientSecret()
	if err != nil {
		return nil, err
	}
	hash, err := HashClientSecret(plaintext)
	if err != nil {
		return nil, err
	}

	row := &model.ServiceCredential{
		ClientID:    clientID,
		DisplayName: input.DisplayName,
		SecretHash:  hash,
		Audiences:   audiencesJSON,
		Scopes:      scopesJSON,
		Status:      model.ServiceCredentialStatusActive,
		OwnerUserID: input.OwnerUserID,
		Description: input.Description,
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}

	view, err := newCredentialView(row)
	if err != nil {
		return nil, err
	}
	return &CreateResult{
		Credential:   row,
		View:         view,
		ClientID:     row.ClientID,
		ClientSecret: plaintext,
	}, nil
}

// List returns all credentials in deterministic (client_id) order, soft-
// deletes filtered out by GORM.
func (s *Service) List() ([]CredentialView, error) {
	var rows []model.ServiceCredential
	if err := s.db.Order("client_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]CredentialView, 0, len(rows))
	for i := range rows {
		v, err := newCredentialView(&rows[i])
		if err != nil {
			return nil, err
		}
		views = append(views, *v)
	}
	return views, nil
}

// GetByClientID looks up a credential row by its client_id. Returns
// ErrCredentialNotFound if missing (soft-deleted rows are invisible) and
// ErrCredentialDisabled if status != active.
func (s *Service) GetByClientID(clientID string) (*model.ServiceCredential, error) {
	if clientID == "" {
		return nil, ErrCredentialNotFound
	}
	var row model.ServiceCredential
	if err := s.db.Where("client_id = ?", clientID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCredentialNotFound
		}
		return nil, err
	}
	if row.Status != model.ServiceCredentialStatusActive {
		return nil, ErrCredentialDisabled
	}
	return &row, nil
}

// VerifySecret loads a credential by client_id and validates the supplied
// plaintext against its bcrypt hash. Returns the credential row on
// success. The credential must be active; both not-found and disabled
// surface as their respective sentinel errors.
//
// Timing: bcrypt comparison is performed in ALL branches (including
// not-found and disabled) to make response timing equivalent across the
// "valid client_id, wrong secret" and "unknown client_id" cases. Without
// this, an attacker can enumerate registered client_ids by measuring the
// ~250ms bcrypt cost-12 delta against the ~1ms DB miss. The dummy hash
// is a constant-format bcrypt output (see dummyClientSecretHash) — the
// underlying plaintext is irrelevant because the comparison is meant to
// fail; only the wall time matters.
func (s *Service) VerifySecret(clientID, clientSecret string) (*model.ServiceCredential, error) {
	row, lookupErr := s.GetByClientID(clientID)

	hashToCompare := []byte(dummyClientSecretHash)
	if row != nil {
		hashToCompare = []byte(row.SecretHash)
	}
	cmpErr := VerifyClientSecret(string(hashToCompare), clientSecret)

	if lookupErr != nil {
		// Return ErrCredentialNotFound or ErrCredentialDisabled.
		// The bcrypt comparison result above is discarded — its only
		// purpose was to consume the same CPU time as the happy path.
		return nil, lookupErr
	}
	if cmpErr != nil {
		return nil, ErrInvalidClientSecret
	}
	return row, nil
}

// RotateSecret regenerates a fresh plaintext secret for the existing
// credential, replaces the stored bcrypt hash, and returns the plaintext
// to the caller exactly once. The old secret is invalidated immediately
// (no grace period — Path D §1.6: rotate = create-and-disable).
func (s *Service) RotateSecret(clientID string) (*CreateResult, error) {
	if clientID == "" {
		return nil, ErrEmptyClientID
	}
	plaintext, err := GenerateClientSecret()
	if err != nil {
		return nil, err
	}
	hash, err := HashClientSecret(plaintext)
	if err != nil {
		return nil, err
	}
	var row model.ServiceCredential
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client_id = ?", clientID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCredentialNotFound
			}
			return err
		}
		return tx.Model(&row).Update("secret_hash", hash).Error
	})
	if err != nil {
		return nil, err
	}
	view, err := newCredentialView(&row)
	if err != nil {
		return nil, err
	}
	return &CreateResult{
		Credential:   &row,
		View:         view,
		ClientID:     row.ClientID,
		ClientSecret: plaintext,
	}, nil
}

// SetStatus flips a credential between active and disabled. Disabled
// credentials cannot issue new tokens. Existing tokens (already-issued
// JWTs) remain valid until natural expiry per Path D §1.6.
func (s *Service) SetStatus(clientID, status string) (*model.ServiceCredential, error) {
	if status != model.ServiceCredentialStatusActive && status != model.ServiceCredentialStatusDisabled {
		return nil, ErrInvalidStatus
	}
	var row model.ServiceCredential
	err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ServiceCredential{}).Where("client_id = ?", clientID).Update("status", status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrCredentialNotFound
		}
		return tx.Where("client_id = ?", clientID).First(&row).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCredentialNotFound
		}
		return nil, err
	}
	return &row, nil
}

// MarkUsed updates last_used_at on the credential row. Best-effort; a
// failure here is logged by the caller but does not fail the request.
func (s *Service) MarkUsed(clientID string, at time.Time) error {
	return s.db.Model(&model.ServiceCredential{}).
		Where("client_id = ?", clientID).
		Update("last_used_at", sql.NullTime{Time: at, Valid: true}).Error
}

// DecodeStringList returns a []string from a JSON datatypes.JSON column
// value. Exported for handler-side use when projecting credentials to
// presentation responses.
func DecodeStringList(raw datatypes.JSON) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	if values == nil {
		return []string{}, nil
	}
	return values, nil
}

func encodeStringList(values []string) (datatypes.JSON, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(encoded), nil
}

func newCredentialView(row *model.ServiceCredential) (*CredentialView, error) {
	if row == nil {
		return nil, nil
	}
	audiences, err := DecodeStringList(row.Audiences)
	if err != nil {
		return nil, err
	}
	scopes, err := DecodeStringList(row.Scopes)
	if err != nil {
		return nil, err
	}
	return &CredentialView{
		ClientID:    row.ClientID,
		DisplayName: row.DisplayName,
		Status:      row.Status,
		OwnerUserID: row.OwnerUserID,
		Description: row.Description,
		Audiences:   audiences,
		Scopes:      scopes,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}
