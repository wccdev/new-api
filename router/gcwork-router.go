package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// gcworkAccessTokenScope is the personal access token scope the GC Work
// control plane needs; it is the existing administrator user-management scope.
const gcworkAccessTokenScope = "user:write"

// SetGcworkRouter adds the GC Work control plane endpoints: provisioning
// users by Casdoor subject and managing their relay tokens.
func SetGcworkRouter(router *gin.Engine) {
	gcworkRouter := router.Group("/api/gcwork")
	gcworkRouter.Use(middleware.RouteTag("api"))
	gcworkRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	gcworkRouter.Use(middleware.AccessTokenAudit())
	gcworkRouter.Use(middleware.GlobalAPIRateLimit())
	gcworkRouter.Use(middleware.DisableCache())
	gcworkRouter.GET("/self/managed-token", middleware.UserAuth(), controller.GcworkGetSelfManagedToken)
	gcworkRouter.Use(middleware.AdminAuth())
	routes := []struct {
		method  string
		path    string
		handler gin.HandlerFunc
	}{
		{"POST", "/users/ensure", controller.GcworkEnsureUser},
		{"POST", "/users/:id/status", controller.GcworkSetUserStatus},
		{"POST", "/users/:id/tokens", controller.GcworkCreateToken},
		{"POST", "/users/:id/managed-token", controller.GcworkEnsureManagedToken},
		{"DELETE", "/users/:id/managed-token", controller.GcworkDeleteManagedToken},
		{"DELETE", "/tokens/:id", controller.GcworkDeleteToken},
	}
	for _, route := range routes {
		middleware.DeclareAccessTokenScopeRoute(route.method, joinPaths(gcworkRouter.BasePath(), route.path), gcworkAccessTokenScope)
		gcworkRouter.Handle(route.method, route.path, route.handler)
	}
}
