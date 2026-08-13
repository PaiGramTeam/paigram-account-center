package authority

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/logging"
	"paigram/internal/middleware"
	"paigram/internal/response"
	serviceaudit "paigram/internal/service/audit"
	"paigram/internal/service/authority"
	pkgerrors "paigram/pkg/errors"
)

type AuthorityHandler struct {
	service *authority.AuthorityService
}

func NewAuthorityHandler(service *authority.AuthorityService) *AuthorityHandler {
	return &AuthorityHandler{service: service}
}

// @Tags      Authority
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Success   200   {object}  response.Response{data=model.Role}
// @Failure   400   {object}  response.Response
// @Failure   401   {object}  response.Response
// @Failure   403   {object}  response.Response
// @Failure   409   {object}  response.Response
// @Failure   500   {object}  response.Response
// @Router    /api/v1/admin/roles [post]
func (h *AuthorityHandler) CreateAuthority(c *gin.Context) {
	var req CreateAuthorityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logging.Error("create authority: invalid request body", zap.Error(err))
		response.BadRequest(c, "参数错误")
		return
	}

	role, err := h.service.CreateAuthority(authority.CreateAuthorityParams{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		PermissionIDs: req.PermissionIDs,
	})
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNameDuplicate) {
			response.Conflict(c, "角色名称已存在")
			return
		}
		logging.Error("create authority failed", zap.Error(err), zap.String("name", req.Name))
		response.InternalServerError(c, "创建角色失败")
		return
	}

	response.Success(c, role)
	h.recordAudit(c, serviceaudit.WriteInput{
		Category:    "authority",
		ActorType:   "admin",
		ActorUserID: authorityActorUserID(c),
		Action:      "authority_create",
		TargetType:  "role",
		TargetID:    strconv.FormatUint(uint64(role.ID), 10),
		Result:      "success",
	})
}

// @Tags      Authority
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  response.Response{data=model.Role}
// @Failure   400  {object}  response.Response
// @Failure   401  {object}  response.Response
// @Failure   403  {object}  response.Response
// @Failure   404  {object}  response.Response
// @Failure   500  {object}  response.Response
// @Router    /api/v1/admin/roles/{id} [get]
func (h *AuthorityHandler) GetAuthority(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的角色ID")
		return
	}

	role, err := h.service.GetAuthority(uint(roleID))
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		logging.Error("get authority failed", zap.Error(err), zap.Uint64("role_id", roleID))
		response.InternalServerError(c, "获取角色失败")
		return
	}

	response.Success(c, role)
}

// @Tags      Authority
// @Security  BearerAuth
// @Produce   json
// @Success   200       {object}  response.Response{data=ListAuthoritiesResponse}
// @Failure   401       {object}  response.Response
// @Failure   403       {object}  response.Response
// @Failure   500       {object}  response.Response
// @Router    /api/v1/admin/roles [get]
func (h *AuthorityHandler) ListAuthorities(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	name := c.Query("name")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	result, err := h.service.ListAuthorities(authority.ListAuthoritiesParams{
		Page:     page,
		PageSize: pageSize,
		Name:     name,
	})
	if err != nil {
		logging.Error("list authorities failed", zap.Error(err))
		response.InternalServerError(c, "获取角色列表失败")
		return
	}

	response.SuccessWithPagination(c, result.Data, int64(result.Total), result.Page, result.PageSize)
}

// @Tags      Authority
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Success   200   {object}  response.Response
// @Failure   400   {object}  response.Response
// @Failure   401   {object}  response.Response
// @Failure   403   {object}  response.Response
// @Failure   404   {object}  response.Response
// @Failure   409   {object}  response.Response
// @Failure   500   {object}  response.Response
// @Router    /api/v1/admin/roles/{id} [put]
// @Router    /api/v1/admin/roles/{id} [patch]
func (h *AuthorityHandler) UpdateAuthority(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的角色ID")
		return
	}

	var req UpdateAuthorityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logging.Error("update authority: invalid request body", zap.Error(err), zap.Uint64("role_id", roleID))
		response.BadRequest(c, "参数错误")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		response.BadRequest(c, "参数错误: name不能为空")
		return
	}

	err = h.service.UpdateAuthority(uint(roleID), authority.UpdateAuthorityParams{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
	})
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		if errors.Is(err, pkgerrors.ErrRoleNameDuplicate) {
			response.Conflict(c, "角色名称已存在")
			return
		}
		if errors.Is(err, pkgerrors.ErrSystemRoleProtect) {
			response.Forbidden(c, "系统角色不可修改")
			return
		}
		logging.Error("update authority failed", zap.Error(err), zap.Uint64("role_id", roleID))
		response.InternalServerError(c, "更新角色失败")
		return
	}

	response.SuccessWithMessage(c, nil, "更新成功")
	h.recordAudit(c, serviceaudit.WriteInput{
		Category:    "authority",
		ActorType:   "admin",
		ActorUserID: authorityActorUserID(c),
		Action:      "authority_update",
		TargetType:  "role",
		TargetID:    strconv.FormatUint(roleID, 10),
		Result:      "success",
	})
}

// @Tags      Authority
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  response.Response
// @Failure   400  {object}  response.Response
// @Failure   401  {object}  response.Response
// @Failure   404  {object}  response.Response
// @Failure   409  {object}  response.Response
// @Failure   500  {object}  response.Response
// @Router    /api/v1/admin/roles/{id} [delete]
func (h *AuthorityHandler) DeleteAuthority(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的角色ID")
		return
	}

	err = h.service.DeleteAuthority(uint(roleID))
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		if errors.Is(err, pkgerrors.ErrSystemRoleProtect) {
			response.Forbidden(c, "系统角色不可删除")
			return
		}
		if errors.Is(err, pkgerrors.ErrRoleInUse) {
			response.Conflict(c, "角色正在使用中,无法删除")
			return
		}
		logging.Error("delete authority failed", zap.Error(err), zap.Uint64("role_id", roleID))
		response.InternalServerError(c, "删除角色失败")
		return
	}

	response.SuccessWithMessage(c, nil, "删除成功")
	h.recordAudit(c, serviceaudit.WriteInput{
		Category:    "authority",
		ActorType:   "admin",
		ActorUserID: authorityActorUserID(c),
		Action:      "authority_delete",
		TargetType:  "role",
		TargetID:    strconv.FormatUint(roleID, 10),
		Result:      "success",
	})
}

// @Tags      Authority
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Success   200   {object}  response.Response
// @Failure   400   {object}  response.Response
// @Failure   401   {object}  response.Response
// @Failure   403   {object}  response.Response
// @Failure   404   {object}  response.Response
// @Failure   500   {object}  response.Response
// @Router    /api/v1/admin/roles/{id}/permissions [put]
func (h *AuthorityHandler) AssignPermissions(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的角色ID")
		return
	}

	var req AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logging.Error("assign permissions: invalid request body", zap.Error(err), zap.Uint64("role_id", roleID))
		response.BadRequest(c, "参数错误")
		return
	}

	err = h.service.AssignPermissions(uint(roleID), req.PermissionIDs)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		if errors.Is(err, pkgerrors.ErrSystemRoleProtect) {
			response.ForbiddenWithCode(c, response.ErrCodePermissionDenied, "系统角色必须保留恢复管理员访问所需的权限", nil)
			return
		}
		logging.Error("assign permissions failed", zap.Error(err), zap.Uint64("role_id", roleID))
		response.InternalServerError(c, "分配权限失败")
		return
	}

	response.SuccessWithMessage(c, nil, "分配成功")
	h.recordAudit(c, serviceaudit.WriteInput{
		Category:    "authority",
		ActorType:   "admin",
		ActorUserID: authorityActorUserID(c),
		Action:      "authority_assign_permissions",
		TargetType:  "role",
		TargetID:    strconv.FormatUint(roleID, 10),
		Result:      "success",
		Metadata: map[string]any{
			"permission_ids": req.PermissionIDs,
		},
	})
}

func (h *AuthorityHandler) recordAudit(c *gin.Context, input serviceaudit.WriteInput) {
	if h == nil || h.service == nil || h.service.DB() == nil {
		return
	}
	input.IP = c.ClientIP()
	input.UserAgent = c.Request.UserAgent()
	_ = serviceaudit.Record(c.Request.Context(), h.service.DB(), input)
}

func authorityActorUserID(c *gin.Context) *uint64 {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == 0 {
		return nil
	}
	return &userID
}

// @Tags      Authority
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  response.Response{data=[]model.Permission}
// @Failure   400  {object}  response.Response
// @Failure   401  {object}  response.Response
// @Failure   403  {object}  response.Response
// @Failure   404  {object}  response.Response
// @Failure   500  {object}  response.Response
// @Router    /api/v1/admin/roles/{id}/permissions [get]
func (h *AuthorityHandler) GetRolePermissions(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的角色ID")
		return
	}

	permissions, err := h.service.GetRolePermissions(uint(roleID))
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		logging.Error("get role permissions failed", zap.Error(err), zap.Uint64("role_id", roleID))
		response.InternalServerError(c, "获取权限失败")
		return
	}

	response.Success(c, permissions)
}

// @Tags      Authority
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  response.Response{data=[]model.Permission}
// @Failure   401  {object}  response.Response
// @Failure   403  {object}  response.Response
// @Failure   500  {object}  response.Response
// @Router    /api/v1/admin/permissions [get]
func (h *AuthorityHandler) ListPermissions(c *gin.Context) {
	permissions, err := h.service.ListPermissions()
	if err != nil {
		logging.Error("list permissions failed", zap.Error(err))
		response.InternalServerError(c, "获取权限列表失败")
		return
	}

	response.Success(c, permissions)
}

// @Tags      Authority
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  response.Response{data=[]AuthorityUserItem}
// @Failure   400  {object}  response.Response
// @Failure   401  {object}  response.Response
// @Failure   403  {object}  response.Response
// @Failure   404  {object}  response.Response
// @Failure   500  {object}  response.Response
// @Router    /api/v1/admin/roles/{id}/users [get]
func (h *AuthorityHandler) GetAuthorityUsers(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的角色ID")
		return
	}

	users, err := h.service.GetAuthorityUsers(uint(roleID))
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		logging.Error("get authority users failed", zap.Error(err), zap.Uint64("role_id", roleID))
		response.InternalServerError(c, "获取角色用户失败")
		return
	}

	items := make([]AuthorityUserItem, 0, len(users))
	for _, user := range users {
		items = append(items, AuthorityUserItem{
			ID:           user.ID,
			DisplayName:  user.DisplayName,
			PrimaryEmail: user.PrimaryEmail,
			AssignedAt:   user.AssignedAt,
			GrantedBy:    user.GrantedBy,
		})
	}

	response.Success(c, items)
}

// @Tags      Authority
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Success   200   {object}  response.Response
// @Failure   400   {object}  response.Response
// @Failure   401   {object}  response.Response
// @Failure   403   {object}  response.Response
// @Failure   404   {object}  response.Response
// @Failure   500   {object}  response.Response
// @Router    /api/v1/admin/roles/{id}/users [put]
func (h *AuthorityHandler) ReplaceAuthorityUsers(c *gin.Context) {
	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的角色ID")
		return
	}

	var req ReplaceAuthorityUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logging.Error("replace authority users: invalid request body", zap.Error(err), zap.Uint64("role_id", roleID))
		response.BadRequest(c, "参数错误")
		return
	}
	if req.UserIDs == nil {
		response.BadRequest(c, "参数错误: user_ids is required")
		return
	}

	currentUserID, exists := middleware.GetUserID(c)
	if !exists {
		response.UnauthorizedWithCode(c, "UNAUTHORIZED", "authentication required", nil)
		return
	}

	err = h.service.ReplaceAuthorityUsers(uint(roleID), req.UserIDs, currentUserID)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrRoleNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		if errors.Is(err, pkgerrors.ErrSystemRoleProtect) {
			response.ForbiddenWithCode(c, response.ErrCodePermissionDenied, "at least one active administrator is required", nil)
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "用户不存在")
			return
		}
		logging.Error("replace authority users failed", zap.Error(err), zap.Uint64("role_id", roleID))
		response.InternalServerError(c, "更新角色用户失败")
		return
	}

	response.SuccessWithMessage(c, nil, "更新成功")
}
