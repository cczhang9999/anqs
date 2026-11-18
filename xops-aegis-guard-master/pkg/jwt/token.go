package jwt

import (
	"errors"
	"strings"

	"xops-aegis-guard/pkg/mistake"
	"xops-aegis-guard/pkg/user"

	jwtgo "github.com/dgrijalva/jwt-go"
)

// TokenData mirrors the SSO token payload used by xops-port-check.
type TokenData struct {
	Data *user.AuthUser `json:"data"`
	jwtgo.StandardClaims
}

// ParseToken reads the SSO user carried by Auth-Token. As in xops-port-check,
// token authenticity is verified by the upstream SSO/gateway rather than this
// application; this layer only extracts the current user for business APIs.
func ParseToken(token string) (*user.AuthUser, error) {
	if token == "" || len(strings.Split(token, ".")) != 3 {
		return nil, mistake.NewStatusUnauthorizedErr()
	}

	parsed, _, err := new(jwtgo.Parser).ParseUnverified(token, &TokenData{})
	if err != nil || parsed == nil {
		return nil, mistake.NewStatusUnauthorizedErr()
	}
	claims, ok := parsed.Claims.(*TokenData)
	if !ok || claims.Data == nil {
		return nil, errors.New("invalid token")
	}
	return claims.Data, nil
}
