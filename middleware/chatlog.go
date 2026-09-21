package middleware

import (
	"github.com/QuantumNous/new-api/chatlog"

	"github.com/gin-gonic/gin"
)

// ChatlogCapture records conversation relay calls when CHATLOG_ENABLED is set.
// Register it inside BodyStorageCleanup; see chatlog.Capture.
func ChatlogCapture() gin.HandlerFunc {
	return chatlog.Capture()
}
