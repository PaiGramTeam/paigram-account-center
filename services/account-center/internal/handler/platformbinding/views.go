package platformbinding

import (
	"time"

	"paigram/internal/model"
)

type BindingView struct {
	ID                  uint64                             `json:"id"`
	Platform            string                             `json:"platform"`
	ExternalAccountKey  any                                `json:"external_account_key"`
	PlatformServiceKey  string                             `json:"platform_service_key"`
	DisplayName         string                             `json:"display_name"`
	Status              model.PlatformAccountBindingStatus `json:"status"`
	StatusReasonCode    string                             `json:"status_reason_code"`
	StatusReasonMessage string                             `json:"status_reason_message"`
	PrimaryProfileID    any                                `json:"primary_profile_id"`
	LastValidatedAt     any                                `json:"last_validated_at"`
	LastSyncedAt        any                                `json:"last_synced_at"`
	CreatedAt           time.Time                          `json:"created_at"`
	UpdatedAt           time.Time                          `json:"updated_at"`
}

type AdminBindingView struct {
	BindingView
	OwnerUserID uint64 `json:"owner_user_id"`
}

type ProfileView struct {
	ID                 uint64    `json:"id"`
	BindingID          uint64    `json:"binding_id"`
	PlatformProfileKey string    `json:"platform_profile_key"`
	GameBiz            string    `json:"game_biz"`
	Region             string    `json:"region"`
	PlayerUID          string    `json:"player_uid"`
	Nickname           string    `json:"nickname"`
	Level              any       `json:"level"`
	IsPrimary          bool      `json:"is_primary"`
	SourceUpdatedAt    any       `json:"source_updated_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ConsumerGrantView struct {
	ID        uint64                    `json:"id,omitempty"`
	BindingID uint64                    `json:"binding_id"`
	Consumer  string                    `json:"consumer"`
	Status    model.ConsumerGrantStatus `json:"status"`
	GrantedBy any                       `json:"granted_by,omitempty"`
	GrantedAt any                       `json:"granted_at,omitempty"`
	RevokedAt any                       `json:"revoked_at"`
	CreatedAt any                       `json:"created_at,omitempty"`
	UpdatedAt any                       `json:"updated_at,omitempty"`
}

func buildMeBindingViews(items []model.PlatformAccountBinding) []BindingView {
	views := make([]BindingView, 0, len(items))
	for index := range items {
		views = append(views, buildMeBindingView(&items[index]))
	}
	return views
}

func buildAdminBindingViews(items []model.PlatformAccountBinding) []AdminBindingView {
	views := make([]AdminBindingView, 0, len(items))
	for index := range items {
		views = append(views, buildAdminBindingView(&items[index]))
	}
	return views
}

func buildMeBindingView(binding *model.PlatformAccountBinding) BindingView {
	return BindingView{
		ID: binding.ID, Platform: binding.Platform, ExternalAccountKey: nullableString(binding.ExternalAccountKey),
		PlatformServiceKey: binding.PlatformServiceKey, DisplayName: binding.DisplayName, Status: binding.Status,
		StatusReasonCode: binding.StatusReasonCode, StatusReasonMessage: binding.StatusReasonMessage,
		PrimaryProfileID: nullableInt64(binding.PrimaryProfileID), LastValidatedAt: nullableTime(binding.LastValidatedAt),
		LastSyncedAt: nullableTime(binding.LastSyncedAt), CreatedAt: binding.CreatedAt, UpdatedAt: binding.UpdatedAt,
	}
}

func buildAdminBindingView(binding *model.PlatformAccountBinding) AdminBindingView {
	return AdminBindingView{BindingView: buildMeBindingView(binding), OwnerUserID: binding.OwnerUserID}
}

func buildProfileViews(items []model.PlatformAccountProfile) []ProfileView {
	views := make([]ProfileView, 0, len(items))
	for _, item := range items {
		views = append(views, ProfileView{
			ID: item.ID, BindingID: item.BindingID, PlatformProfileKey: item.PlatformProfileKey,
			GameBiz: item.GameBiz, Region: item.Region, PlayerUID: item.PlayerUID, Nickname: item.Nickname,
			Level: nullableInt64(item.Level), IsPrimary: item.IsPrimary, SourceUpdatedAt: nullableTime(item.SourceUpdatedAt),
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	return views
}

func buildGrantViews(items []model.ConsumerGrant) []ConsumerGrantView {
	views := make([]ConsumerGrantView, 0, len(items))
	for index := range items {
		views = append(views, buildGrantView(&items[index]))
	}
	return views
}

func buildGrantView(item *model.ConsumerGrant) ConsumerGrantView {
	view := ConsumerGrantView{
		BindingID: item.BindingID, Consumer: item.Consumer, Status: item.Status, RevokedAt: nullableTime(item.RevokedAt),
	}
	if item.ID != 0 {
		view.ID = item.ID
		view.GrantedBy = nullableInt64(item.GrantedBy)
		view.GrantedAt = item.GrantedAt
		view.CreatedAt = item.CreatedAt
		view.UpdatedAt = item.UpdatedAt
	}
	return view
}
