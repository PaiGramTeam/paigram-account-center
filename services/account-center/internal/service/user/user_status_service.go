package user

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"paigram/internal/dberror"
	"paigram/internal/model"
	pkgerrors "paigram/pkg/errors"
)

func (s *UserService) UpdateUserStatus(userID uint64, status model.UserStatus) (*model.User, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("load user for status update: %w", err)
	}
	if err := s.db.Model(&user).Update("status", status).Error; err != nil {
		if dberror.IsActiveAdministratorGuardViolation(err) {
			return nil, pkgerrors.ErrSystemRoleProtect
		}
		return nil, fmt.Errorf("update user status: %w", err)
	}
	user.Status = status
	return &user, nil
}

func (s *UserService) HardDeleteUser(userID uint64) error {
	return s.deleteUser(userID, true)
}
