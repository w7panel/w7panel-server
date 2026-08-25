package panelauth

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

const (
	TokenUsePanel       = "panel"
	TokenUseExternalAPI = "external-api"
	Issuer              = "w7panel"
)

var ErrInvalidToken = errors.New("invalid panel token")

type Claims struct {
	Username       string `json:"username"`
	PermissionName string `json:"permissionName"`
	Role           string `json:"role"`
	TokenUse       string `json:"tokenUse"`
	jwt.RegisteredClaims
}

type Principal struct {
	Username       string
	PermissionName string
	Role           string
	TokenUse       string
}

func Issue(principal Principal, ttl time.Duration) (string, error) {
	if principal.Username == "" || principal.TokenUse == "" {
		return "", fmt.Errorf("%w: principal is incomplete", ErrInvalidToken)
	}
	if ttl <= 0 {
		return "", fmt.Errorf("%w: ttl must be positive", ErrInvalidToken)
	}
	now := time.Now()
	claims := Claims{
		Username:       principal.Username,
		PermissionName: principal.PermissionName,
		Role:           principal.Role,
		TokenUse:       principal.TokenUse,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   principal.Username,
			Audience:  []string{"panel-api"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(signingKey())
}

func Parse(raw string) (*Principal, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return signingKey(), nil
	}, jwt.WithIssuer(Issuer), jwt.WithAudience("panel-api"))
	if err != nil || token == nil || !token.Valid || claims.Subject == "" || claims.Subject != claims.Username {
		return nil, ErrInvalidToken
	}
	if claims.TokenUse != TokenUsePanel && claims.TokenUse != TokenUseExternalAPI {
		return nil, ErrInvalidToken
	}
	return &Principal{Username: claims.Username, PermissionName: claims.PermissionName, Role: claims.Role, TokenUse: claims.TokenUse}, nil
}

func signingKey() []byte {
	key := strings.TrimSpace(os.Getenv("PANEL_AUTH_SIGNING_KEY"))
	if key == "" {
		key = strings.TrimSpace(facade.Config.GetString("panel_auth.signing_key"))
	}
	if key == "" {
		// aeskey is already deployment-specific configuration. Hashing prevents it
		// from being reused directly by another symmetric function.
		key = facade.Config.GetString("app.aeskey")
	}
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}
