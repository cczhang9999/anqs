package middleware

import (
	"xops-aegis-guard/pkg/jwt"
	"xops-aegis-guard/pkg/user"

	"github.com/gin-gonic/gin"
)

const ctxLoginUserKey = "ctxLoginUserKey"

// AuthUser 按照 port-check 的约定把当前用户放入 Gin context。
func AuthUser(ctx *gin.Context) {
	u := whoAmI(ctx)
	ctx.Set(ctxLoginUserKey, u)
}

// whoAmI parses the SSO Auth-Token. Internal callers remain compatible with
// xops-port-check: a missing or malformed token falls back to 内部调用.
func whoAmI(ctx *gin.Context) *user.AuthUser {
	u, err := jwt.ParseToken(ctx.GetHeader("Auth-Token"))
	if err == nil && u != nil {
		return u
	}
	return &user.AuthUser{
		Email:    "内部调用",
		UserId:   1,
		Username: "内部调用",
		Is_super: 0,
	}
}

// GetUserFromContext 从 context 获取当前用户。
func GetUserFromContext(ctx *gin.Context) (authUser *user.AuthUser, exists bool) {
	value, exists := ctx.Get(ctxLoginUserKey)
	if !exists {
		return nil, false
	}
	u, ok := value.(*user.AuthUser)
	return u, ok
}
