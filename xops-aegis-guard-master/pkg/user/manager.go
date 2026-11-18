package user

// AuthUser 是接口层透传的当前登录用户。
// 业务请求统一携带该对象，后续需要做权限校验或审计时无需再改接口签名。
type AuthUser struct {
	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Is_super int    `json:"is_super"`
}
