package authority

import (
	"time"

	"paigram/internal/response"
	serviceauthority "paigram/internal/service/authority"
)

type CreateAuthorityRequest struct {
	Name          string `json:"name" binding:"required,min=2,max=50"`
	Description   string `json:"description" binding:"max=200"`
	PermissionIDs []uint `json:"permission_ids"`
}

type UpdateAuthorityRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=2,max=50"`
	Description *string `json:"description" binding:"omitempty,max=200"`
}

type AssignPermissionsRequest struct {
	PermissionIDs []uint `json:"permission_ids" binding:"required"`
}

type ReplaceAuthorityUsersRequest struct {
	UserIDs []uint64 `json:"user_ids"`
}

type ListAuthoritiesResponse struct {
	Items      []serviceauthority.RoleWithPermissions `json:"items"`
	Pagination *response.PaginationMeta               `json:"pagination"`
}

type AuthorityUserItem struct {
	ID           uint64    `json:"id"`
	DisplayName  string    `json:"display_name"`
	PrimaryEmail string    `json:"primary_email"`
	AssignedAt   time.Time `json:"assigned_at"`
	GrantedBy    uint64    `json:"granted_by"`
}
