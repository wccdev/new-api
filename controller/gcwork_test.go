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

	t.Run("ensure refuses a subject bound to an administrator", func(t *testing.T) {
		require.NoError(t, model.DB.Model(admin).Update("oidc_id", "casdoor-admin").Error)
		payload := call(writer, http.MethodPost, "/api/gcwork/users/ensure", `{"external_id":"casdoor-admin","username":"root2"}`)
		assert.False(t, payload.Success)
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

	t.Run("refuses tokens for administrators and past expiry times", func(t *testing.T) {
		valid := `{"name":"x","expired_time":-1,"unlimited_quota":true}`
		assert.False(t, call(writer, http.MethodPost, fmt.Sprintf("/api/gcwork/users/%d/tokens", admin.Id), valid).Success)
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
