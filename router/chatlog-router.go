package router

import (
	"github.com/QuantumNous/new-api/chatlog"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// SetChatlogRouter adds the conversation viewer endpoints used by the usage
// logs page.
func SetChatlogRouter(router *gin.Engine) {
	chatlogRouter := router.Group("/api/chatlog")
	chatlogRouter.Use(middleware.RouteTag("api"))
	chatlogRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	chatlogRouter.Use(middleware.GlobalAPIRateLimit())
	chatlogRouter.Use(middleware.DisableCache())
	chatlogRouter.GET("/self/:request_id", middleware.UserAuth(), func(c *gin.Context) {
		chatlog.Conversation(c, false)
	})
	chatlogRouter.GET("/:request_id", middleware.AdminAuth(), func(c *gin.Context) {
		chatlog.Conversation(c, true)
	})
}
