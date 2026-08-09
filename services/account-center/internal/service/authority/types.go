package authority

import "time"

type CreateAuthorityParams struct {
	Name          string
	DisplayName   string
	Description   string
	PermissionIDs []uint
}

type UpdateAuthorityParams struct {
	Name        *string
	DisplayName *string
	Description *string
}

type ListAuthoritiesParams struct {
	Page     int
	PageSize int
	Name     string
}

type ListAuthoritiesResult struct {
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Data     []RoleWithPermissions `json:"data"`
}

type RoleWithPermissions struct {
	ID          uint             `json:"id"`
	Name        string           `json:"name"`
	DisplayName string           `json:"display_name"`
	Description string           `json:"description"`
	IsSystem    bool             `json:"is_system"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Permissions []PermissionInfo `json:"permissions"`
}

type PermissionInfo struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description"`
}
