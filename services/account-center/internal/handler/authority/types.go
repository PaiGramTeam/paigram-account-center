package authority

import (
	"time"

	"paigram/internal/response"
	serviceauthority "paigram/internal/service/authority"
)

type CreateAuthorityRequest struct {
	Name          string `json:"name" binding:"required,min=2,max=50" minLength:"2" maxLength:"50"`
	DisplayName   string `json:"display_name,omitempty" maxLength:"100"`
	Description   string `json:"description,omitempty" binding:"max=200" maxLength:"200"`
	PermissionIDs []uint `json:"permission_ids,omitempty"`
}

type UpdateAuthorityRequest struct {
	Name        *string `json:"name,omitempty" binding:"omitempty,min=2,max=50" minLength:"2" maxLength:"50"`
	DisplayName *string `json:"display_name,omitempty" maxLength:"100"`
	Description *string `json:"description,omitempty" binding:"omitempty,max=200" maxLength:"200"`
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
