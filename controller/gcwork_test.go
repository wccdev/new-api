package controller

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newGcworkTestRouter mirrors router.SetGcworkRouter, so the access token
// route rules keyed by gin full path apply unchanged.
func newGcworkTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(middleware.RequestId(), middleware.AccessTokenAudit())
	group := router.Group("/api/gcwork", middleware.AdminAuth())
	for _, route := range []struct {
		method, path string
		handler      gin.HandlerFunc
	}{
		{http.MethodPost, "/users/ensure", GcworkEnsureUser},
		{http.MethodPost, "/users/:id/status", GcworkSetUserStatus},
		{http.MethodPost, "/users/:id/tokens", GcworkCreateToken},
		{http.MethodPost, "/users/:id/managed-token", GcworkEnsureManagedToken},
		{http.MethodDelete, "/users/:id/managed-token", GcworkDeleteManagedToken},
		{http.MethodDelete, "/tokens/:id", GcworkDeleteToken},
	} {
		middleware.DeclareAccessTokenScopeRoute(route.method, "/api/gcwork"+route.path, "user:write")
		group.Handle(route.method, route.path, route.handler)
	}
	return router
}

func TestGcworkControlPlaneEndpoints(t *testing.T) {
	admin, _ := setupAccessTokenAudit(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	// Every goroutine must share the one in-memory SQLite database.
	sqlDB.SetMaxOpenConns(1)
	router := newGcworkTestRouter()
	expiresAt := time.Now().Add(time.Hour).Unix()
	writer, _ := createScopedAccessToken(t, admin.Id, expiresAt, "user:write")
	reader, _ := createScopedAccessToken(t, admin.Id, expiresAt, "user:read")
	call := func(credential, method, path, body string) tokenAPIResponse {
		response := accessTokenRequest(router, method, path, credential, "", body)
		var payload tokenAPIResponse
		if response.Code == http.StatusOK {
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		}
		return payload
	}
	ensure := `{"external_id":"casdoor-sub-1","username":"alice","display_name":"Alice Zhang","email":"alice@example.com"}`

	t.Run("requires the user:write scope", func(t *testing.T) {
		response := accessTokenRequest(router, http.MethodPost, "/api/gcwork/users/ensure", reader, "", ensure)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})

	var userID int
	t.Run("ensure creates one user per subject, also under concurrency", func(t *testing.T) {
		ids := make([]int, 8)
		var wg sync.WaitGroup
		for i := range ids {
			wg.Go(func() {
				var data struct {
					ID int `json:"id"`
				}
				payload := call(writer, http.MethodPost, "/api/gcwork/users/ensure", ensure)
				if payload.Success && common.Unmarshal(payload.Data, &data) == nil {
					ids[i] = data.ID
				}
			})
		}
		wg.Wait()
		userID = ids[0]
		require.NotZero(t, userID)
		for _, id := range ids {
			assert.Equal(t, userID, id)
		}
		var users []model.User
		require.NoError(t, model.DB.Where("oidc_id = ?", "casdoor-sub-1").Find(&users).Error)
		require.Len(t, users, 1)
		assert.Equal(t, "alice", users[0].Username)
		assert.Equal(t, "Alice Zhang", users[0].DisplayName)
		assert.Equal(t, "alice@example.com", users[0].Email)
		assert.Equal(t, common.RoleCommonUser, users[0].Role)
		assert.Equal(t, common.QuotaForNewUser, users[0].Quota)
	})

	t.Run("ensure falls back to a derived username and drops a taken email", func(t *testing.T) {
		payload := call(writer, http.MethodPost, "/api/gcwork/users/ensure", `{"external_id":"casdoor-sub-2","username":"alice","email":"alice@example.com"}`)
		require.True(t, payload.Success, payload.Message)
		var user model.User
		require.NoError(t, model.DB.Where("oidc_id = ?", "casdoor-sub-2").First(&user).Error)
		assert.True(t, strings.HasPrefix(user.Username, "gcw_"))
		assert.Len(t, user.Username, model.UserNameMaxLength)
		assert.Empty(t, user.Email)
	})

	root := model.User{Username: "gcwork-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "gcwork-root", OidcId: "casdoor-root"}
	require.NoError(t, model.DB.Create(&root).Error)
	t.Run("ensure returns an administrator but refuses the root user", func(t *testing.T) {
		require.NoError(t, model.DB.Model(admin).Update("oidc_id", "casdoor-admin").Error)
		payload := call(writer, http.MethodPost, "/api/gcwork/users/ensure", `{"external_id":"casdoor-admin","username":"admin2"}`)
		require.True(t, payload.Success, payload.Message)
		assert.JSONEq(t, fmt.Sprintf(`{"id":%d}`, admin.Id), string(payload.Data))
		assert.False(t, call(writer, http.MethodPost, "/api/gcwork/users/ensure", `{"external_id":"casdoor-root","username":"root2"}`).Success)
	})

	var tokenID int
	t.Run("creates a relay token owned by the user and returns its key", func(t *testing.T) {
		body := fmt.Sprintf(`{"name":"gc-work device","expired_time":%d,"remain_quota":0,"unlimited_quota":true}`, expiresAt)
		payload := call(writer, http.MethodPost, fmt.Sprintf("/api/gcwork/users/%d/tokens", userID), body)
		require.True(t, payload.Success, payload.Message)
		var data struct {
			ID  int    `json:"id"`
			Key string `json:"key"`
		}
		require.NoError(t, common.Unmarshal(payload.Data, &data))
		require.True(t, strings.HasPrefix(data.Key, "sk-"))
		token, err := model.GetTokenById(data.ID)
		require.NoError(t, err)
		assert.Equal(t, userID, token.UserId)
		assert.Equal(t, "sk-"+token.Key, data.Key)
		tokenID = data.ID
	})

	t.Run("issues tokens to administrators, not to the root user or with past expiry times", func(t *testing.T) {
		valid := `{"name":"x","expired_time":-1,"unlimited_quota":true}`
		assert.True(t, call(writer, http.MethodPost, fmt.Sprintf("/api/gcwork/users/%d/tokens", admin.Id), valid).Success)
		assert.False(t, call(writer, http.MethodPost, fmt.Sprintf("/api/gcwork/users/%d/tokens", root.Id), valid).Success)
		expired := `{"name":"x","expired_time":1,"unlimited_quota":true}`
		assert.False(t, call(writer, http.MethodPost, fmt.Sprintf("/api/gcwork/users/%d/tokens", userID), expired).Success)
	})

	t.Run("disables and enables the user", func(t *testing.T) {
		path := fmt.Sprintf("/api/gcwork/users/%d/status", userID)
		require.True(t, call(writer, http.MethodPost, path, `{"enabled":false}`).Success)
		user, err := model.GetUserById(userID, false)
		require.NoError(t, err)
		assert.Equal(t, common.UserStatusDisabled, user.Status)
		require.True(t, call(writer, http.MethodPost, path, `{"enabled":true}`).Success)
		user, err = model.GetUserById(userID, false)
		require.NoError(t, err)
		assert.Equal(t, common.UserStatusEnabled, user.Status)
		assert.False(t, call(writer, http.MethodPost, fmt.Sprintf("/api/gcwork/users/%d/status", admin.Id), `{"enabled":false}`).Success)
	})

	t.Run("deletes a token, and deleting a missing token succeeds", func(t *testing.T) {
		path := fmt.Sprintf("/api/gcwork/tokens/%d", tokenID)
		require.True(t, call(writer, http.MethodDelete, path, "").Success)
		_, err := model.GetTokenById(tokenID)
		require.Error(t, err)
		assert.True(t, call(writer, http.MethodDelete, path, "").Success)
	})
}

func TestGcworkManagedToken(t *testing.T) {
	admin, _ := setupAccessTokenAudit(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}, &model.GcworkManagedToken{}))
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	router := newGcworkTestRouter()
	writer, _ := createScopedAccessToken(t, admin.Id, time.Now().Add(time.Hour).Unix(), "user:write")
	user := model.User{Username: "managed", DisplayName: "managed", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "managed"}
	require.NoError(t, model.DB.Create(&user).Error)
	path := fmt.Sprintf("/api/gcwork/users/%d/managed-token", user.Id)
	type issued struct {
		ID  int    `json:"id"`
		Key string `json:"key"`
	}
	ensure := func() issued {
		response := accessTokenRequest(router, http.MethodPost, path, writer, "", "")
		var payload tokenAPIResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		require.True(t, payload.Success, payload.Message)
		var data issued
		require.NoError(t, common.Unmarshal(payload.Data, &data))
		return data
	}
	deleteManaged := func() tokenAPIResponse {
		response := accessTokenRequest(router, http.MethodDelete, path, writer, "", "")
		var payload tokenAPIResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		return payload
	}

	t.Run("returns the same permanent key on every call, also under concurrency", func(t *testing.T) {
		results := make([]issued, 8)
		var wg sync.WaitGroup
		for i := range results {
			wg.Go(func() { results[i] = ensure() })
		}
		wg.Wait()
		for _, result := range results {
			assert.Equal(t, results[0], result)
		}
		assert.True(t, strings.HasPrefix(results[0].Key, "sk-"))
		token, err := model.GetTokenById(results[0].ID)
		require.NoError(t, err)
		assert.Equal(t, int64(-1), token.ExpiredTime)
		assert.True(t, token.UnlimitedQuota)
		assert.Equal(t, model.GcworkManagedTokenName, token.Name)
		count, err := model.CountUserTokens(user.Id)
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("ignores the per-user token limit", func(t *testing.T) {
		other := model.User{Username: "limited", DisplayName: "limited", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "limited"}
		require.NoError(t, model.DB.Create(&other).Error)
		limit := operation_setting.GetTokenSetting().MaxUserTokens
		operation_setting.GetTokenSetting().MaxUserTokens = 0
		t.Cleanup(func() { operation_setting.GetTokenSetting().MaxUserTokens = limit })
		token, err := model.EnsureGcworkManagedToken(other.Id)
		require.NoError(t, err)
		assert.Positive(t, token.Id)
	})

	t.Run("enables a disabled token again", func(t *testing.T) {
		first := ensure()
		require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", first.ID).Update("status", common.TokenStatusDisabled).Error)
		assert.Equal(t, first, ensure())
		token, err := model.GetTokenById(first.ID)
		require.NoError(t, err)
		assert.Equal(t, common.TokenStatusEnabled, token.Status)
	})

	t.Run("the user can neither change the token nor read its key", func(t *testing.T) {
		managed := ensure()
		own := model.Token{UserId: user.Id, Name: "own", Key: "own-key-for-managed-test", ExpiredTime: -1}
		require.NoError(t, own.Insert())
		tokens := gin.New()
		group := tokens.Group("/api/token", func(c *gin.Context) { c.Set("id", user.Id) }, middleware.TokenOperationAudit(), middleware.GcworkManagedTokenGuard())
		group.GET("/:id", GetToken)
		group.POST("/:id/key", GetTokenKey)
		group.PUT("/", UpdateToken)
		group.DELETE("/:id", DeleteToken)
		group.POST("/batch", DeleteTokenBatch)
		group.POST("/batch/keys", GetTokenKeysBatch)
		request := func(method, target, body string) tokenAPIResponse {
			response := accessTokenRequest(tokens, method, target, "unused", "", body)
			var payload tokenAPIResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload), response.Body.String())
			return payload
		}
		for _, attempt := range []struct{ method, target, body string }{
			{http.MethodDelete, fmt.Sprintf("/api/token/%d", managed.ID), ""},
			{http.MethodPost, fmt.Sprintf("/api/token/%d/key", managed.ID), ""},
			{http.MethodPut, "/api/token/", fmt.Sprintf(`{"id":%d,"name":"renamed","expired_time":-1,"unlimited_quota":true}`, managed.ID)},
			{http.MethodPut, "/api/token/?status_only=true", fmt.Sprintf(`{"id":%d,"status":2}`, managed.ID)},
			{http.MethodPost, "/api/token/batch", fmt.Sprintf(`{"ids":[%d,%d]}`, own.Id, managed.ID)},
			{http.MethodPost, "/api/token/batch/keys", fmt.Sprintf(`{"ids":[%d,%d]}`, own.Id, managed.ID)},
		} {
			payload := request(attempt.method, attempt.target, attempt.body)
			assert.False(t, payload.Success, attempt.target)
			assert.Contains(t, payload.Message, "managed by GC Work", attempt.target)
		}
		token, err := model.GetTokenById(managed.ID)
		require.NoError(t, err)
		assert.Equal(t, model.GcworkManagedTokenName, token.Name)
		assert.Equal(t, common.TokenStatusEnabled, token.Status)
		_, err = model.GetTokenById(own.Id)
		require.NoError(t, err)

		assert.True(t, request(http.MethodGet, fmt.Sprintf("/api/token/%d", managed.ID), "").Success)
		assert.True(t, request(http.MethodPost, fmt.Sprintf("/api/token/%d/key", own.Id), "").Success)
		assert.True(t, request(http.MethodPut, "/api/token/", fmt.Sprintf(`{"id":%d,"name":"renamed","expired_time":-1,"unlimited_quota":true}`, own.Id)).Success)
		assert.True(t, request(http.MethodDelete, fmt.Sprintf("/api/token/%d", own.Id), "").Success)
	})

	t.Run("deleting stops the key and the next ensure issues another; deleting again succeeds", func(t *testing.T) {
		first := ensure()
		require.True(t, deleteManaged().Success)
		_, err := model.GetTokenById(first.ID)
		require.Error(t, err)
		assert.True(t, deleteManaged().Success)
		assert.NotEqual(t, first.Key, ensure().Key)
	})

	t.Run("replaces a marker whose token is gone", func(t *testing.T) {
		first := ensure()
		token, err := model.GetTokenById(first.ID)
		require.NoError(t, err)
		require.NoError(t, token.Delete())
		assert.NotEqual(t, first.Key, ensure().Key)
	})

	t.Run("refuses the root user", func(t *testing.T) {
		root := model.User{Username: "managed-root", DisplayName: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "managed-root"}
		require.NoError(t, model.DB.Create(&root).Error)
		response := accessTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/gcwork/users/%d/managed-token", root.Id), writer, "", "")
		var payload tokenAPIResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		assert.False(t, payload.Success)
	})
}

func TestGcworkEnsureUserMatchesCasdoorBindings(t *testing.T) {
	admin, _ := setupAccessTokenAudit(t)
	require.NoError(t, model.DB.AutoMigrate(&model.CustomOAuthProvider{}))
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	office := model.CustomOAuthProvider{Name: "Casdoor office", Slug: "casdoor-oa"}
	research := model.CustomOAuthProvider{Name: "Casdoor research", Slug: "casdoor-rd"}
	require.NoError(t, model.DB.Create(&office).Error)
	require.NoError(t, model.DB.Create(&research).Error)
	t.Setenv("GCWORK_OAUTH_PROVIDERS", "casdoor-oa, casdoor-rd")
	router := newGcworkTestRouter()
	writer, _ := createScopedAccessToken(t, admin.Id, time.Now().Add(time.Hour).Unix(), "user:write")
	createUser := func(username string) model.User {
		user := model.User{Username: username, DisplayName: username, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: username}
		require.NoError(t, model.DB.Create(&user).Error)
		return user
	}
	bind := func(user model.User, provider model.CustomOAuthProvider, subject string) {
		require.NoError(t, model.DB.Create(&model.UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: subject}).Error)
	}
	ensure := func(subject, username string) (int, bool) {
		response := accessTokenRequest(router, http.MethodPost, "/api/gcwork/users/ensure", writer, "",
			fmt.Sprintf(`{"external_id":%q,"username":%q}`, subject, username))
		require.Equal(t, http.StatusOK, response.Code)
		var payload tokenAPIResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		var data struct {
			ID int `json:"id"`
		}
		if payload.Success {
			require.NoError(t, common.Unmarshal(payload.Data, &data))
		}
		return data.ID, payload.Success
	}
	bindings := func(subject string) []model.UserOAuthBinding {
		var found []model.UserOAuthBinding
		require.NoError(t, model.DB.Where("provider_user_id = ?", subject).Find(&found).Error)
		return found
	}

	t.Run("returns the user bound through any configured provider", func(t *testing.T) {
		user := createUser("A0001")
		bind(user, research, "sub-1")
		id, ok := ensure("sub-1", "A0001")
		require.True(t, ok)
		assert.Equal(t, user.Id, id)
	})

	t.Run("adopts an unbound user with the Casdoor username once", func(t *testing.T) {
		user := createUser("A0002")
		id, ok := ensure("sub-2", "A0002")
		require.True(t, ok)
		assert.Equal(t, user.Id, id)
		require.Len(t, bindings("sub-2"), 1)
		assert.Equal(t, office.Id, bindings("sub-2")[0].ProviderId)
		again, ok := ensure("sub-2", "A0002")
		require.True(t, ok)
		assert.Equal(t, user.Id, again)
	})

	t.Run("does not adopt a user bound to another subject", func(t *testing.T) {
		user := createUser("A0003")
		bind(user, office, "someone-else")
		id, ok := ensure("sub-3", "A0003")
		require.True(t, ok)
		assert.NotEqual(t, user.Id, id)
		created, err := model.GetUserById(id, false)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(created.Username, "gcw_"))
		assert.Empty(t, created.OidcId)
	})

	t.Run("creates a new user bound to the first provider", func(t *testing.T) {
		id, ok := ensure("sub-4", "A0004")
		require.True(t, ok)
		created, err := model.GetUserById(id, false)
		require.NoError(t, err)
		assert.Equal(t, "A0004", created.Username)
		require.Len(t, bindings("sub-4"), 1)
		assert.Equal(t, id, bindings("sub-4")[0].UserId)
		assert.Equal(t, office.Id, bindings("sub-4")[0].ProviderId)
	})

	t.Run("refuses an unknown provider slug", func(t *testing.T) {
		t.Setenv("GCWORK_OAUTH_PROVIDERS", "missing")
		_, ok := ensure("sub-5", "A0005")
		assert.False(t, ok)
	})
}
