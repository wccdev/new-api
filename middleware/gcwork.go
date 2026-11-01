package middleware

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GcworkManagedTokenGuard refuses user-facing requests that change a token
// GC Work manages or read its key. It covers the single and batch routes of
// /api/token; listing and reading the masked token stay available.
func GcworkManagedTokenGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		ids := gcworkTargetTokenIds(c)
		managed, err := model.AnyGcworkManagedToken(ids)
		if err != nil {
			common.ApiError(c, err)
			c.Abort()
			return
		}
		if managed {
			common.ApiErrorT(c, "This API key is managed by GC Work and cannot be changed or viewed")
			c.Abort()
			return
		}
		c.Next()
	}
}

// gcworkTargetTokenIds names the tokens the request acts on. A malformed body
// names none, and the handler then rejects the request itself.
func gcworkTargetTokenIds(c *gin.Context) []int {
	switch c.Request.Method + " " + c.FullPath() {
	case "DELETE /api/token/:id", "POST /api/token/:id/key":
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			return nil
		}
		return []int{id}
	case "PUT /api/token/":
		var body struct {
			Id int `json:"id"`
		}
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			return nil
		}
		return []int{body.Id}
	case "POST /api/token/batch", "POST /api/token/batch/keys":
		var body struct {
			Ids []int `json:"ids"`
		}
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			return nil
		}
		return body.Ids
	}
	return nil
}
