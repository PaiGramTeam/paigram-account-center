package authority

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"paigram/internal/model"
	"paigram/pkg/errors"
)

type authorityCasbinSyncer interface {
	SyncAuthorityPermissionPolicies(roleID uint) error
	DeleteAuthorityPolicies(roleID uint) error
}

type AuthorityService struct {
	db            *gorm.DB
	casbinService authorityCasbinSyncer
}

func (s *AuthorityService) DB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *AuthorityService) CreateAuthority(params CreateAuthorityParams) (*model.Role, error) {
	var role *model.Role
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		displayName = params.Name
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Role{}).Where("name = ?", params.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.ErrRoleNameDuplicate
		}

		role = &model.Role{
			Name:        params.Name,
			DisplayName: displayName,
			Description: params.Description,
			IsSystem:    false,
		}
		if err := tx.Create(role).Error; err != nil {
			return err
		}

		if len(params.PermissionIDs) > 0 {
			for _, permID := range params.PermissionIDs {
				rp := &model.RolePermission{
					RoleID:       role.ID,
					PermissionID: uint64(permID),
				}
				if err := tx.Create(rp).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	if s.casbinService != nil && len(params.PermissionIDs) > 0 {
		if err := s.casbinService.SyncAuthorityPermissionPolicies(uint(role.ID)); err != nil {
			rollbackErr := s.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Where("role_id = ?", role.ID).Delete(&model.RolePermission{}).Error; err != nil {
					return err
				}
				return tx.Delete(&model.Role{}, role.ID).Error
			})
			if rollbackErr != nil {
				return nil, fmt.Errorf("sync authority policies: %w; rollback created authority: %v", err, rollbackErr)
			}
			return nil, err
		}
	}

	return role, nil
}

func (s *AuthorityService) UpdateAuthority(roleID uint, params UpdateAuthorityParams) error {
	var role model.Role
	if err := s.db.First(&role, uint64(roleID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.ErrRoleNotFound
		}
		return err
	}

	if role.IsSystem {
		return errors.ErrSystemRoleProtect
	}

	if params.Name != nil && *params.Name != role.Name {
		var count int64
		if err := s.db.Model(&model.Role{}).Where("name = ? AND id != ?", *params.Name, uint64(roleID)).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.ErrRoleNameDuplicate
		}
		role.Name = *params.Name
	}

	if params.Description != nil {
		role.Description = *params.Description
	}
	if params.DisplayName != nil {
		role.DisplayName = *params.DisplayName
	}

	return s.db.Save(&role).Error
}

func (s *AuthorityService) DeleteAuthority(roleID uint) error {
	var role model.Role
	if err := s.db.First(&role, uint64(roleID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.ErrRoleNotFound
		}
		return err
	}

	if role.IsSystem {
		return errors.ErrSystemRoleProtect
	}

	var count int64
	if err := s.db.Model(&model.UserRole{}).Where("role_id = ?", uint64(roleID)).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.ErrRoleInUse
	}

	previousPermissions, err := s.loadRolePermissionIDs(roleID)
	if err != nil {
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", uint64(roleID)).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&role).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	if s.casbinService == nil {
		return nil
	}

	if err := s.casbinService.DeleteAuthorityPolicies(roleID); err != nil {
		rollbackErr := s.restoreDeletedAuthority(role, previousPermissions)
		if rollbackErr != nil {
			return fmt.Errorf("delete authority policies: %w; restore deleted authority: %v", err, rollbackErr)
		}
		return err
	}

	return nil
}

func (s *AuthorityService) GetAuthority(roleID uint) (*model.Role, error) {
	var role model.Role
	if err := s.db.Preload("Permissions").First(&role, uint64(roleID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrRoleNotFound
		}
		return nil, err
	}
	return &role, nil
}

func (s *AuthorityService) ListAuthorities(params ListAuthoritiesParams) (*ListAuthoritiesResult, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 10
	}

	var roles []model.Role
	query := s.db.Model(&model.Role{})

	if params.Name != "" {
		query = query.Where("name LIKE ?", "%"+params.Name+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (params.Page - 1) * params.PageSize
	if err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(params.PageSize).Find(&roles).Error; err != nil {
		return nil, err
	}

	data := make([]RoleWithPermissions, len(roles))
	for i, role := range roles {
		data[i] = RoleWithPermissions{
			ID:          uint(role.ID),
			Name:        role.Name,
			DisplayName: role.DisplayName,
			Description: role.Description,
			IsSystem:    role.IsSystem,
			CreatedAt:   role.CreatedAt,
			UpdatedAt:   role.UpdatedAt,
			Permissions: []PermissionInfo{},
		}
	}

	return &ListAuthoritiesResult{
		Total:    int(total),
		Page:     params.Page,
		PageSize: params.PageSize,
		Data:     data,
	}, nil
}

func (s *AuthorityService) AssignPermissions(roleID uint, permissionIDs []uint) error {
	previousPermissions, err := s.loadRolePermissionIDs(roleID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.ErrRoleNotFound
		}
		return err
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var role model.Role
		if err := tx.First(&role, uint64(roleID)).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return errors.ErrRoleNotFound
			}
			return err
		}

		if err := tx.Where("role_id = ?", uint64(roleID)).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		for _, permID := range permissionIDs {
			rp := &model.RolePermission{
				RoleID:       uint64(roleID),
				PermissionID: uint64(permID),
			}
			if err := tx.Create(rp).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	if s.casbinService == nil {
		return nil
	}

	if err := s.casbinService.SyncAuthorityPermissionPolicies(roleID); err != nil {
		rollbackErr := s.replaceRolePermissions(roleID, previousPermissions)
		if rollbackErr != nil {
			return fmt.Errorf("sync authority policies: %w; restore previous permissions: %v", err, rollbackErr)
		}
		return err
	}

	return nil
}

func (s *AuthorityService) GetRolePermissions(roleID uint) ([]model.Permission, error) {
	if err := s.ensureAuthorityExists(s.db, roleID); err != nil {
		return nil, err
	}

	var permissions []model.Permission

	err := s.db.Table("permissions").
		Joins("INNER JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", uint64(roleID)).
		Find(&permissions).Error

	return permissions, err
}

// ListPermissions returns the complete assignable permission catalog.
func (s *AuthorityService) ListPermissions() ([]model.Permission, error) {
	var permissions []model.Permission
	err := s.db.Order("name ASC").Find(&permissions).Error
	return permissions, err
}

func (s *AuthorityService) loadRolePermissionIDs(roleID uint) ([]uint64, error) {
	var role model.Role
	if err := s.db.Select("id").First(&role, uint64(roleID)).Error; err != nil {
		return nil, err
	}

	var permissionIDs []uint64
	err := s.db.Model(&model.RolePermission{}).Where("role_id = ?", uint64(roleID)).Pluck("permission_id", &permissionIDs).Error
	if err != nil {
		return nil, err
	}

	return permissionIDs, nil
}

func (s *AuthorityService) replaceRolePermissions(roleID uint, permissionIDs []uint64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", uint64(roleID)).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		for _, permissionID := range permissionIDs {
			rp := model.RolePermission{RoleID: uint64(roleID), PermissionID: permissionID}
			if err := tx.Create(&rp).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *AuthorityService) restoreDeletedAuthority(role model.Role, permissionIDs []uint64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Model(&model.Role{}).Where("id = ?", role.ID).Update("deleted_at", nil).Error; err != nil {
			return err
		}

		for _, permissionID := range permissionIDs {
			rp := model.RolePermission{RoleID: role.ID, PermissionID: permissionID}
			if err := tx.Create(&rp).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
