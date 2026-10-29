package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GC Work control plane endpoints. They are machine-to-machine calls made with
// an administrator personal access token holding the user:write scope, so they
// require no interactive security proof. Administrators use GC Work like other
// employees and may receive relay tokens, which reach models only; enabling or
// disabling users and adopting accounts by username stay limited to common
// users, and the root user is never a target.

const gcworkExternalIDMaxLength = 128

// gcworkEnsureLock serializes provisioning on this node; across nodes the
// unique OAuth binding and username indexes decide.
var gcworkEnsureLock sync.Mutex

type gcworkEnsureUserRequest struct {
	ExternalID  string `json:"external_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

// gcworkOAuthProviders resolves GCWORK_OAUTH_PROVIDERS, the comma-separated
// slugs of the custom OAuth providers that sign users in through the same
// Casdoor as GC Work. The first one receives the binding of a new user.
func gcworkOAuthProviders() ([]*model.CustomOAuthProvider, error) {
	providers := []*model.CustomOAuthProvider{}
	for slug := range strings.SplitSeq(os.Getenv("GCWORK_OAUTH_PROVIDERS"), ",") {
		if slug = strings.TrimSpace(slug); slug == "" {
			continue
		}
		provider, err := model.GetCustomOAuthProviderBySlug(slug)
		if err != nil {
			return nil, fmt.Errorf("GCWORK_OAUTH_PROVIDERS names unknown provider %q: %w", slug, err)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

// GcworkEnsureUser returns the user bound to a Casdoor subject, creating it in
// the default group with the default quota on first use. With
// GCWORK_OAUTH_PROVIDERS set, the subject is matched against those providers'
// bindings, an unbound user whose username equals the Casdoor username is
// adopted, and a new user gets a binding to the first provider, so NewAPI web
// sign-in through Casdoor reaches the same account. Without it, the subject is
// stored as the OIDC id.
func GcworkEnsureUser(c *gin.Context) {
	var req gcworkEnsureUserRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ExternalID == "" || len(req.ExternalID) > gcworkExternalIDMaxLength {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	providers, err := gcworkOAuthProviders()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	gcworkEnsureLock.Lock()
	defer gcworkEnsureLock.Unlock()

	if user, err := gcworkUserByExternalID(providers, req.ExternalID); err != nil || user != nil {
		gcworkEnsureResult(c, user, err)
		return
	}
	if len(providers) > 0 && req.Username != "" {
		var existing model.User
		err := model.DB.Where("username = ?", req.Username).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, err)
			return
		}
		if err == nil && existing.OidcId == "" && existing.Role < common.RoleAdminUser {
			var bound int64
			ids := make([]int, len(providers))
			for i, provider := range providers {
				ids[i] = provider.Id
			}
			if err := model.DB.Model(&model.UserOAuthBinding{}).Where("user_id = ? AND provider_id IN ?", existing.Id, ids).Count(&bound).Error; err != nil {
				common.ApiError(c, err)
				return
			}
			if bound == 0 {
				binding := model.UserOAuthBinding{UserId: existing.Id, ProviderId: providers[0].Id, ProviderUserId: req.ExternalID}
				if err := model.CreateUserOAuthBinding(&binding); err != nil {
					// Another node may have bound the subject meanwhile.
					user, lookupErr := gcworkUserByExternalID(providers, req.ExternalID)
					if lookupErr == nil && user == nil {
						lookupErr = err
					}
					gcworkEnsureResult(c, user, lookupErr)
					return
				}
				recordManageAuditFor(c, existing.Id, "gcwork.user.adopt", map[string]any{"id": existing.Id, "username": existing.Username, "provider": providers[0].Slug})
				common.ApiSuccess(c, gin.H{"id": existing.Id})
				return
			}
		}
	}

	candidates := []string{}
	if req.Username != "" && len(req.Username) <= model.UserNameMaxLength {
		candidates = append(candidates, req.Username)
	}
	digest := sha256.Sum256([]byte(req.ExternalID))
	candidates = append(candidates, "gcw_"+hex.EncodeToString(digest[:8]))
	displayName := []rune(req.DisplayName)
	if len(displayName) == 0 {
		displayName = []rune(req.Username)
	}
	email := model.NormalizeEmail(req.Email)
	if email != "" && model.EnsureEmailAvailable(email, 0) != nil {
		email = ""
	}

	var insertErr error
	for _, username := range candidates {
		user := model.User{
			Username:    username,
			DisplayName: string(displayName[:min(len(displayName), 20)]),
			Email:       email,
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
		}
		if len(providers) == 0 {
			user.OidcId = req.ExternalID
		}
		insert := func(tx *gorm.DB) error {
			if err := user.InsertWithTx(tx, 0); err != nil {
				return err
			}
			if len(providers) == 0 {
				return nil
			}
			return model.CreateUserOAuthBindingWithTx(tx, &model.UserOAuthBinding{UserId: user.Id, ProviderId: providers[0].Id, ProviderUserId: req.ExternalID})
		}
		insertErr = model.DB.Transaction(insert)
		if errors.Is(insertErr, model.ErrEmailAlreadyTaken) {
			user.Id, user.Email, email = 0, "", ""
			insertErr = model.DB.Transaction(insert)
		}
		if insertErr == nil {
			user.FinalizeOAuthUserCreation(0)
			recordManageAuditFor(c, user.Id, "gcwork.user.ensure", map[string]any{"id": user.Id, "username": user.Username})
			common.ApiSuccess(c, gin.H{"id": user.Id})
			return
		}
		// Another node may have created the user meanwhile.
		if existing, err := gcworkUserByExternalID(providers, req.ExternalID); err != nil || existing != nil {
			gcworkEnsureResult(c, existing, err)
			return
		}
		user.Id = 0
	}
	common.ApiError(c, insertErr)
}

// gcworkUserByExternalID finds the user bound to a Casdoor subject through the
// configured providers' OAuth bindings, then through the OIDC id.
func gcworkUserByExternalID(providers []*model.CustomOAuthProvider, externalID string) (*model.User, error) {
	for _, provider := range providers {
		user, err := model.GetUserByOAuthBinding(provider.Id, externalID)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	var user model.User
	err := model.DB.Where("oidc_id = ?", externalID).Order("id").First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func gcworkEnsureResult(c *gin.Context, user *model.User, err error) {
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user.Role >= common.RoleRootUser {
		common.ApiErrorT(c, "No permission to update users of same or higher permission level")
		return
	}
	common.ApiSuccess(c, gin.H{"id": user.Id})
}

// gcworkTargetUser loads the user named by the :id path parameter whose role
// is below maxRole, or writes the error response and returns nil.
func gcworkTargetUser(c *gin.Context, maxRole int) *model.User {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return nil
	}
	user := model.User{Id: id}
	if err := model.DB.Where(&user).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorT(c, "User does not exist")
		} else {
			common.ApiError(c, err)
		}
		return nil
	}
	if user.Role >= maxRole {
		common.ApiErrorT(c, "No permission to update users of same or higher permission level")
		return nil
	}
	return &user
}

// GcworkSetUserStatus enables or disables a common user. Disabling stops every
// relay token of the user at the next request.
func GcworkSetUserStatus(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Enabled == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	user := gcworkTargetUser(c, common.RoleAdminUser)
	if user == nil {
		return
	}
	user.Status = common.UserStatusDisabled
	if *req.Enabled {
		user.Status = common.UserStatusEnabled
	}
	if err := user.Update(false); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.InvalidateUserTokensCache(user.Id); err != nil {
		common.SysLog(common.LogText("failed to invalidate tokens cache for user %d: %s", user.Id, err.Error()))
	}
	recordManageAuditFor(c, user.Id, "gcwork.user.status", map[string]any{"id": user.Id, "username": user.Username, "enabled": *req.Enabled})
	common.ApiSuccess(c, gin.H{"status": user.Status})
}

type gcworkCreateTokenRequest struct {
	Name           string `json:"name"`
	ExpiredTime    int64  `json:"expired_time"`
	RemainQuota    int    `json:"remain_quota"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
}

// GcworkCreateToken creates a relay token owned by a non-root user, applying the
// validation of the user-facing token API, and returns its full key once.
func GcworkCreateToken(c *gin.Context) {
	var req gcworkCreateTokenRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if len(req.Name) > 50 {
		common.ApiErrorT(c, "Token name is too long")
		return
	}
	if req.ExpiredTime != -1 && req.ExpiredTime <= common.GetTimestamp() {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if !req.UnlimitedQuota {
		if req.RemainQuota < 0 {
			common.ApiErrorT(c, "Quota value cannot be negative")
			return
		}
		if maxQuotaValue := maxTokenQuota(); req.RemainQuota > maxQuotaValue {
			common.ApiErrorT(c, "Quota value exceeds valid range, maximum is {{max}}", map[string]any{"max": logger.FormatQuota(maxQuotaValue)})
			return
		}
	}
	user := gcworkTargetUser(c, common.RoleRootUser)
	if user == nil {
		return
	}
	count, err := model.CountUserTokens(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if maxTokens := operation_setting.GetMaxUserTokens(); int(count) >= maxTokens {
		common.ApiErrorT(c, "Maximum number of tokens reached ({{max}})", map[string]any{"max": maxTokens})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorT(c, "Failed to generate token")
		common.SysLog(common.LogText("failed to generate token key: %s", err.Error()))
		return
	}
	now := common.GetTimestamp()
	token := model.Token{
		UserId:              user.Id,
		Name:                req.Name,
		Key:                 key,
		CreatedTime:         now,
		AccessedTime:        now,
		ExpiredTime:         req.ExpiredTime,
		RemainQuota:         req.RemainQuota,
		UnlimitedQuota:      req.UnlimitedQuota,
		AllowUserQuotaQuery: true,
	}
	if err := token.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "gcwork.token.create", map[string]any{"id": token.Id, "user_id": user.Id, "name": token.Name})
	common.ApiSuccess(c, gin.H{"id": token.Id, "key": "sk-" + key})
}

// GcworkDeleteToken deletes a relay token owned by a non-root user. A token that
// no longer exists counts as deleted.
func GcworkDeleteToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	token, err := model.GetTokenById(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiSuccess(c, nil)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var owner model.User
	if err := model.DB.Unscoped().Select("id", "role").First(&owner, token.UserId).Error; err == nil && owner.Role >= common.RoleRootUser {
		common.ApiErrorT(c, "No permission to update users of same or higher permission level")
		return
	}
	if err := token.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, token.UserId, "gcwork.token.delete", map[string]any{"id": token.Id, "user_id": token.UserId})
	common.ApiSuccess(c, nil)
}
