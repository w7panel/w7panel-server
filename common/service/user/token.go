package user

import (
	"fmt"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

const TokenIssuer = "w7panel"

type Claims struct {
	Username       string `json:"username"`
	UserMode       string `json:"userMode"`
	Role           string `json:"role"`
	PermissionName string `json:"permissionName"`
	jwtv5.RegisteredClaims
}

func SignToken(u *User, seconds int64) (string, error) {
	now := time.Now()
	claims := Claims{
		Username:       u.Name,
		UserMode:       u.Spec.UserMode,
		Role:           role(u),
		PermissionName: u.Spec.PermissionName,
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    TokenIssuer,
			Subject:   u.Name,
			Audience:  jwtv5.ClaimStrings{u.Name},
			IssuedAt:  jwtv5.NewNumericDate(now),
			NotBefore: jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(now.Add(time.Duration(seconds) * time.Second)),
		},
	}
	return jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).SignedString(secret())
}

func ParseToken(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwtv5.ParseWithClaims(token, claims, func(token *jwtv5.Token) (interface{}, error) {
		if token.Method != jwtv5.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secret(), nil
	}, jwtv5.WithIssuer(TokenIssuer))
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func secret() []byte {
	key := facade.Config.GetString("app.aeskey")
	if key == "" {
		key = "w7panel"
	}
	return []byte(key)
}
