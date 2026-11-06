package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
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
// slugs of the custom OAuth providers that sign users in through the Casdoor
// organizations GC Work serves. The first one is the organization GC Work
// itself signs users in with and receives the bindings GC Work creates.
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
// GCWORK_OAUTH_PROVIDERS set, the first provider is the organization GC Work
// signs users in with, and its subjects are bound to that provider. The
// subject is matched against all those providers' bindings; otherwise an
// unbound user whose username equals the Casdoor username is adopted, or the
// account that gcworkEmailOwner names for the email gets the binding, and only
// then is a new user created, so NewAPI web sign-in through any of these
// organizations reaches the same account. Without it, the subject is stored as
// the OIDC id.
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
			if err := model.DB.Model(&model.UserOAuthBinding{}).Where("user_id = ? AND provider_id IN ?", existing.Id, gcworkProviderIds(providers)).Count(&bound).Error; err != nil {
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
	if email := model.NormalizeEmail(req.Email); len(providers) > 1 && email != "" {
		owner, reason, err := gcworkEmailOwner(gcworkProviderIds(providers), providers[0].Id, email)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if owner != nil && reason == "" {
			binding := model.UserOAuthBinding{UserId: owner.Id, ProviderId: providers[0].Id, ProviderUserId: req.ExternalID}
			if err := model.CreateUserOAuthBinding(&binding); err != nil {
				// Another node may have bound the subject meanwhile.
				user, lookupErr := gcworkUserByExternalID(providers, req.ExternalID)
				if lookupErr == nil && user == nil {
					lookupErr = err
				}
				gcworkEnsureResult(c, user, lookupErr)
				return
			}
			notificationFailed := service.NotifyAccountSecurityChange(service.AccountMailLanguage(c, owner.Id), owner.Email, i18n.MsgSecurityEventLoginLinked, map[string]any{"Provider": providers[0].Name}) != nil
			recordManageAuditFor(c, owner.Id, "gcwork.user.link_email", map[string]any{"id": owner.Id, "username": owner.Username, "provider": providers[0].Slug, "notification_failed": notificationFailed})
			common.ApiSuccess(c, gin.H{"id": owner.Id})
			return
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

func gcworkProviderIds(providers []*model.CustomOAuthProvider) []int {
	ids := make([]int, len(providers))
	for i, provider := range providers {
		ids[i] = provider.Id
	}
	return ids
}

// gcworkEmailOwner finds the account that a not yet bound subject of
// providerId belongs to, judged by email: the only account holding the email,
// a common user bound to another of the GC Work providers and not to
// providerId. The GC Work providers are separate Casdoor organizations whose
// accounts are provisioned by administrators, which is what makes their email
// trustworthy. A user with a non-empty reason holds the email but does not
// qualify; a nil user means no single account holds it.
func gcworkEmailOwner(groupIds []int, providerId int, email string) (*model.User, string, error) {
	user, err := model.GetUniqueUserByEmail(email)
	if errors.Is(err, model.ErrEmailNotFound) || errors.Is(err, model.ErrEmailAmbiguous) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if user.Role >= common.RoleAdminUser {
		return user, "administrator", nil
	}
	var bindings []model.UserOAuthBinding
	if err := model.DB.Where("user_id = ? AND provider_id IN ?", user.Id, groupIds).Find(&bindings).Error; err != nil {
		return nil, "", err
	}
	if len(bindings) == 0 {
		return user, "no_casdoor_binding", nil
	}
	if slices.ContainsFunc(bindings, func(b model.UserOAuthBinding) bool { return b.ProviderId == providerId }) {
		return user, "provider_bound_to_another_subject", nil
	}
	return user, "", nil
}

// linkGcworkOAuthLogin handles a web sign-in through one of the
// GCWORK_OAUTH_PROVIDERS whose subject is not bound to that provider yet. The
// providers are applications of one Casdoor server, whose subjects are unique
// across its organizations, and Casdoor may answer through one application
// with the session of another; so the account the subject is bound to through
// another of the providers signs in as is. Otherwise the subject is linked to
// the account gcworkEmailOwner names. A nil user and error means the sign-in is
// not such a case and registration proceeds as usual.
func linkGcworkOAuthLogin(c *gin.Context, provider oauth.Provider, oauthUser *oauth.OAuthUser) (*model.User, error) {
	generic, ok := provider.(*oauth.GenericOAuthProvider)
	if !ok {
		return nil, nil
	}
	providers, err := gcworkOAuthProviders()
	if err != nil {
		common.SysError(common.LogText("[OAuth] GC Work account linking skipped: %s", err.Error()))
		return nil, nil
	}
	providerId := generic.GetProviderId()
	groupIds := gcworkProviderIds(providers)
	if len(groupIds) < 2 || !slices.Contains(groupIds, providerId) {
		return nil, nil
	}
	for _, id := range groupIds {
		user, err := model.GetUserByOAuthBinding(id, oauthUser.ProviderUserID)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	email := model.NormalizeEmail(oauthUser.Email)
	if email == "" {
		return nil, nil
	}
	user, reason, err := gcworkEmailOwner(groupIds, providerId, email)
	if err != nil || user == nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		// The caller rejects the disabled account; nothing is linked to it.
		return user, nil
	}

	params := map[string]any{"provider": generic.GetConfig().Slug, "provider_user_id": oauthUser.ProviderUserID, "gcwork_email_link": true}
	audit := func(success bool) {
		params["success"] = success
		model.RecordOperationAuditLog(user.Id, user.Role, auditContentEN("user.binding_bind", params), c.ClientIP(), "user.binding_bind", params, nil, &model.AuditRequestInfo{
			Method: c.Request.Method, Route: c.FullPath(), Path: c.FullPath(), Status: c.Writer.Status(), Success: success,
		}, c)
	}
	if reason == "" {
		state, err := model.GetUserVerificationState(user.Id)
		if err != nil {
			return nil, err
		}
		if state.HasTwoFA || state.HasPasskey {
			// Linking before the second factor is checked would let the new
			// subject attach itself even when that check then fails.
			reason = "second_factor_enabled"
		}
	}
	if reason != "" {
		params["reason"] = reason
		audit(false)
		return nil, &OAuthLegacyBindingNotConfirmedError{}
	}

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		return model.CreateUserOAuthBindingWithTx(tx, &model.UserOAuthBinding{UserId: user.Id, ProviderId: providerId, ProviderUserId: oauthUser.ProviderUserID})
	})
	if err != nil {
		return nil, err
	}
	params["notification_failed"] = service.NotifyAccountSecurityChange(service.AccountMailLanguage(c, user.Id), user.Email, i18n.MsgSecurityEventLoginLinked, map[string]any{"Provider": provider.GetName()}) != nil
	audit(true)
	return user, nil
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

// GcworkEnsureManagedToken returns the one permanent relay token GC Work manages
// for a non-root user, creating it on first use. Every device of the user
// receives the same key.
func GcworkEnsureManagedToken(c *gin.Context) {
	user := gcworkTargetUser(c, common.RoleRootUser)
	if user == nil {
		return
	}
	gcworkEnsureLock.Lock()
	defer gcworkEnsureLock.Unlock()
	token, err := model.EnsureGcworkManagedToken(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": token.Id, "key": "sk-" + token.GetFullKey()})
}

// GcworkDeleteManagedToken deletes the managed token of a non-root user, so its
// key stops working on every device. A user without one succeeds.
func GcworkDeleteManagedToken(c *gin.Context) {
	user := gcworkTargetUser(c, common.RoleRootUser)
	if user == nil {
		return
	}
	gcworkEnsureLock.Lock()
	defer gcworkEnsureLock.Unlock()
	if err := model.DeleteGcworkManagedToken(user.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "gcwork.managed_token.delete", map[string]any{"user_id": user.Id})
	common.ApiSuccess(c, nil)
}

// GcworkGetSelfManagedToken tells the console which of the signed-in user's
// tokens GC Work manages; token_id is 0 when there is none.
func GcworkGetSelfManagedToken(c *gin.Context) {
	id, err := model.GcworkManagedTokenId(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"token_id": id})
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
