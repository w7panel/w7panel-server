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
	ConsoleID      string `json:"consoleId,omitempty"`
	CVMName        string `json:"cvmName,omitempty"`
	K3KNamespace   string `json:"k3kNamespace,omitempty"`
	jwt.RegisteredClaims
}

type Principal struct {
	Username       string
	PermissionName string
	Role           string
	TokenUse       string
	ConsoleID      string
	CVMName        string
	K3KNamespace   string
}

func audience(p Principal) []string {
	return []string{p.Username, p.Role, p.ConsoleID, p.CVMName, p.K3KNamespace, "https://kubernetes.default.svc.cluster.local", "k3s"}
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
		ConsoleID:      principal.ConsoleID,
		CVMName:        principal.CVMName,
		K3KNamespace:   principal.K3KNamespace,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   principal.Username,
			Audience:  audience(principal),
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
	}, jwt.WithIssuer(Issuer))
	if err != nil || token == nil || !token.Valid || claims.Subject == "" || claims.Subject != claims.Username {
		return nil, ErrInvalidToken
	}
	if len(claims.Audience) != 7 || claims.Audience[0] != claims.Username || claims.Audience[5] != "https://kubernetes.default.svc.cluster.local" || claims.Audience[6] != "k3s" {
		return nil, ErrInvalidToken
	}
	if claims.TokenUse != TokenUsePanel && claims.TokenUse != TokenUseExternalAPI {
		return nil, ErrInvalidToken
	}
	return &Principal{Username: claims.Username, PermissionName: claims.PermissionName, Role: claims.Role, TokenUse: claims.TokenUse, ConsoleID: claims.ConsoleID, CVMName: claims.CVMName, K3KNamespace: claims.K3KNamespace}, nil
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
