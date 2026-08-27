package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"paigram/internal/config"
	"paigram/internal/handler/shared"
	"paigram/internal/model"
	piiutil "paigram/internal/utils/pii"
)

var errOAuthIdentityIssuerInvalid = errors.New("oauth identity issuer is invalid")

type oauthIdentity struct {
	Issuer  string
	Subject string
}

func resolveOAuthIdentity(
	provider string,
	providerConfig config.OAuthProviderConfig,
	claims *oidcIDTokenClaims,
	userInfo *oauthUserInfo,
) (oauthIdentity, error) {
	issuer, err := resolveOAuthIdentityIssuer(provider, providerConfig, claims)
	if err != nil {
		return oauthIdentity{}, err
	}

	var subject string
	if claims != nil {
		subject = strings.TrimSpace(claims.Subject)
		if subject == "" {
			return oauthIdentity{}, fmt.Errorf("%w: verified token subject is empty", errOAuthIdentityIssuerInvalid)
		}
		if userInfo != nil && userInfo.Subject != "" && strings.TrimSpace(userInfo.Subject) != subject {
			return oauthIdentity{}, fmt.Errorf("%w: userinfo subject does not match verified token", errOAuthIdentityIssuerInvalid)
		}
	} else if userInfo != nil {
		subject = strings.TrimSpace(userInfo.Subject)
		if subject == "" {
			subject = strings.TrimSpace(userInfo.ID)
		}
	}
	if subject == "" {
		return oauthIdentity{}, fmt.Errorf("%w: provider subject is empty", errOAuthIdentityIssuerInvalid)
	}

	return oauthIdentity{Issuer: issuer, Subject: subject}, nil
}

func resolveOAuthIdentityIssuer(
	provider string,
	providerConfig config.OAuthProviderConfig,
	claims *oidcIDTokenClaims,
) (string, error) {
	if claims != nil {
		issuer, err := validateExternalIdentityIssuer(claims.Issuer)
		if err != nil {
			return "", err
		}
		if canonical, ok := canonicalBuiltInIdentityIssuer(provider); ok {
			if issuer != canonical {
				return "", fmt.Errorf("%w: built-in provider issuer does not match canonical issuer", errOAuthIdentityIssuerInvalid)
			}
			return canonical, nil
		}
		return issuer, nil
	}

	if canonical, ok := canonicalBuiltInIdentityIssuer(provider); ok {
		return canonical, nil
	}
	return validateExternalIdentityIssuer(providerConfig.Issuer)
}

func canonicalBuiltInIdentityIssuer(provider string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case string(model.LoginTypeGoogle):
		return model.GoogleIdentityIssuer, true
	case string(model.LoginTypeGithub):
		return model.GitHubIdentityIssuer, true
	case string(model.LoginTypeTelegram):
		return model.TelegramIdentityIssuer, true
	default:
		return "", false
	}
}

func validateExternalIdentityIssuer(raw string) (string, error) {
	issuer := strings.TrimSpace(raw)
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: expected an HTTPS issuer URL", errOAuthIdentityIssuerInvalid)
	}
	return issuer, nil
}

func emailValue(email *model.UserEmail) string {
	if email == nil {
		return ""
	}
	return email.Email
}

type oauthLoginResult struct {
	user              model.User
	emailRecord       *model.UserEmail
	sessionWithTokens *SessionWithTokens
}

func (h *Handler) bindOAuthLoginMethod(provider string, stateRecord model.UserOAuthState, userInfo *oauthUserInfo, tokenResp *oauthTokenResponse, now time.Time) error {
	if !stateRecord.UserID.Valid || stateRecord.UserID.Int64 <= 0 {
		return errMissingBindUser
	}

	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&model.User{}, stateRecord.UserID.Int64).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errMissingBindUser
			}
			return err
		}

		credential, err := buildOAuthCredential(uint64(stateRecord.UserID.Int64), provider, userInfo.Issuer, userInfo.ID, tokenResp, now)
		if err != nil {
			return err
		}

		var existing model.UserCredential
		err = tx.Where("user_id = ? AND provider = ?", stateRecord.UserID.Int64, provider).First(&existing).Error
		if err == nil {
			if existing.Issuer != userInfo.Issuer || existing.ProviderAccountID != userInfo.ID {
				return errProviderRebindConflict
			}

			existing.TokenExpiry = credential.TokenExpiry
			existing.Scopes = credential.Scopes
			existing.LastSyncAt = credential.LastSyncAt
			existing.AccessToken = credential.AccessToken
			existing.RefreshToken = credential.RefreshToken
			return tx.Save(&existing).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		err = tx.Where("issuer = ? AND provider_account_id = ?", userInfo.Issuer, userInfo.ID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(credential).Error
		}
		if err != nil {
			return err
		}
		if existing.UserID != uint64(stateRecord.UserID.Int64) {
			return errProviderAlreadyBound
		}

		existing.TokenExpiry = credential.TokenExpiry
		existing.Scopes = credential.Scopes
		existing.LastSyncAt = credential.LastSyncAt
		existing.AccessToken = credential.AccessToken
		existing.RefreshToken = credential.RefreshToken
		return tx.Save(&existing).Error
	})
}

func (h *Handler) completeOAuthLogin(provider string, userInfo *oauthUserInfo, tokenResp *oauthTokenResponse, now time.Time, clientIP, userAgent string) (*oauthLoginResult, error) {
	result := &oauthLoginResult{}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var credential model.UserCredential
		credErr := tx.Where("issuer = ? AND provider_account_id = ?", userInfo.Issuer, userInfo.ID).First(&credential).Error
		if credErr != nil && !errors.Is(credErr, gorm.ErrRecordNotFound) {
			return credErr
		}

		if errors.Is(credErr, gorm.ErrRecordNotFound) {
			result.user = model.User{
				PrimaryLoginType: loginTypeForOAuthProvider(provider),
				Status:           model.UserStatusActive,
			}
			if err := tx.Create(&result.user).Error; err != nil {
				return err
			}

			displayName := strings.TrimSpace(userInfo.Name)
			if displayName == "" {
				displayName = fmt.Sprintf("%s_user_%s", provider, userInfo.ID)
			}

			profile := model.UserProfile{
				UserID:      result.user.ID,
				DisplayName: displayName,
				AvatarURL:   userInfo.Picture,
				Locale:      "en_US",
			}
			if err := tx.Create(&profile).Error; err != nil {
				return err
			}

			if email := strings.TrimSpace(strings.ToLower(userInfo.Email)); email != "" {
				var existingEmail model.UserEmail
				emailExists := tx.Where("email = ?", email).First(&existingEmail).Error == nil

				if emailExists {
					log.Printf("[OAuth] Email conflict: %s from %s provider already exists for user_id=%d", piiutil.MaskEmail(email), provider, existingEmail.UserID)
				} else {
					emailModel := model.UserEmail{
						UserID:    result.user.ID,
						Email:     email,
						IsPrimary: true,
					}
					if userInfo.EmailVerified {
						emailModel.VerifiedAt = shared.MakeNullTime(now)
					}
					if err := tx.Create(&emailModel).Error; err != nil {
						return err
					}
					result.emailRecord = &emailModel
				}
			}

			newCredential, err := buildOAuthCredential(result.user.ID, provider, userInfo.Issuer, userInfo.ID, tokenResp, now)
			if err != nil {
				return err
			}
			if err := tx.Create(newCredential).Error; err != nil {
				return err
			}
		} else {
			if err := tx.First(&result.user, credential.UserID).Error; err != nil {
				return err
			}

			updatedCredential, err := buildOAuthCredential(result.user.ID, provider, userInfo.Issuer, userInfo.ID, tokenResp, now)
			if err != nil {
				return err
			}
			credential.TokenExpiry = updatedCredential.TokenExpiry
			credential.Scopes = updatedCredential.Scopes
			credential.LastSyncAt = updatedCredential.LastSyncAt
			credential.AccessToken = updatedCredential.AccessToken
			credential.RefreshToken = updatedCredential.RefreshToken

			if err := tx.Save(&credential).Error; err != nil {
				return err
			}

			if email := strings.TrimSpace(strings.ToLower(userInfo.Email)); email != "" {
				var userEmail model.UserEmail
				err := tx.Where("user_id = ? AND email = ?", result.user.ID, email).First(&userEmail).Error
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						var conflictEmail model.UserEmail
						conflict := tx.Where("email = ?", email).First(&conflictEmail).Error == nil

						if conflict {
							log.Printf("[OAuth] Email conflict on update: %s from %s provider already exists for user_id=%d (current user_id=%d)", piiutil.MaskEmail(email), provider, conflictEmail.UserID, result.user.ID)
						} else {
							userEmail = model.UserEmail{
								UserID:    result.user.ID,
								Email:     email,
								IsPrimary: false,
							}
							if userInfo.EmailVerified {
								userEmail.VerifiedAt = shared.MakeNullTime(now)
							}
							if err := tx.Create(&userEmail).Error; err != nil {
								return err
							}
							result.emailRecord = &userEmail
						}
					} else {
						return err
					}
				} else if userInfo.EmailVerified && !userEmail.VerifiedAt.Valid {
					userEmail.VerifiedAt = shared.MakeNullTime(now)
					if err := tx.Save(&userEmail).Error; err != nil {
						return err
					}
					result.emailRecord = &userEmail
				} else {
					result.emailRecord = &userEmail
				}
			}
		}

		updates := map[string]interface{}{
			"last_login_at": shared.MakeNullTime(now),
		}
		if result.user.Status == model.UserStatusPending {
			updates["status"] = model.UserStatusActive
		}
		if err := tx.Model(&model.User{}).Where("id = ?", result.user.ID).Updates(updates).Error; err != nil {
			return err
		}

		var err error
		result.sessionWithTokens, err = h.issueSession(tx, result.user.ID, clientIP, userAgent)
		if err != nil {
			return err
		}

		return h.recordLoginAudit(tx, model.LoginAudit{
			UserID:    sql.NullInt64{Int64: int64(result.user.ID), Valid: true},
			Provider:  provider,
			Success:   true,
			ClientIP:  clientIP,
			UserAgent: userAgent,
			Message:   "oauth login success",
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func buildOAuthCredential(userID uint64, provider, issuer, providerAccountID string, tokenResp *oauthTokenResponse, now time.Time) (*model.UserCredential, error) {
	tokenExpiry := shared.ClearNullTime()
	if tokenResp.ExpiresIn > 0 {
		tokenExpiry = shared.MakeNullTime(now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second))
	}

	credential := &model.UserCredential{
		UserID:            userID,
		Provider:          provider,
		Issuer:            issuer,
		ProviderAccountID: providerAccountID,
		TokenExpiry:       tokenExpiry,
		Scopes:            strings.TrimSpace(tokenResp.Scope),
		LastSyncAt:        shared.MakeNullTime(now),
	}
	if err := credential.SetAccessToken(tokenResp.AccessToken); err != nil {
		return nil, fmt.Errorf("encrypt access token: %w", err)
	}
	if err := credential.SetRefreshToken(tokenResp.RefreshToken); err != nil {
		return nil, fmt.Errorf("encrypt refresh token: %w", err)
	}
	return credential, nil
}

func loginTypeForOAuthProvider(provider string) model.LoginType {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case string(model.LoginTypeGoogle):
		return model.LoginTypeGoogle
	case string(model.LoginTypeGithub):
		return model.LoginTypeGithub
	case string(model.LoginTypeTelegram):
		return model.LoginTypeTelegram
	default:
		return model.LoginType(provider)
	}
}
