package oidc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/w7panel/w7panel/common/service/k8s"
	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type DirectAuthorizeRequest struct {
	Username            string `json:"username" form:"username"`
	Password            string `json:"password" form:"password"`
	ResponseType        string `json:"response_type" form:"response_type"`
	ClientID            string `json:"client_id" form:"client_id"`
	RedirectURI         string `json:"redirect_uri" form:"redirect_uri"`
	Scope               string `json:"scope" form:"scope"`
	State               string `json:"state" form:"state"`
	Nonce               string `json:"nonce" form:"nonce"`
	ResponseMode        string `json:"response_mode" form:"response_mode"`
	CodeChallenge       string `json:"code_challenge" form:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method" form:"code_challenge_method"`
	Prompt              string `json:"prompt" form:"prompt"`
}

type authRequest struct {
	ID            string
	CreationDate  time.Time
	ApplicationID string
	CallbackURI   string
	TransferState string
	Prompt        []string
	UserID        string
	Scopes        []string
	ResponseType  zitadeloidc.ResponseType
	ResponseMode  zitadeloidc.ResponseMode
	Nonce         string
	CodeChallenge *zitadeloidc.CodeChallenge

	done     bool
	authTime time.Time
}

func (s *Server) Login(ctx context.Context, id, username, password string) error {
	userID, err := s.authenticate(ctx, username, password)
	if err != nil {
		return err
	}
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	req, ok := s.authRequests[id]
	if !ok {
		return errors.New("授权请求不存在或已过期")
	}
	req.UserID = userID
	req.done = true
	req.authTime = time.Now()
	return nil
}

func (s *Server) CreateDirectAuthorizationCode(ctx context.Context, req DirectAuthorizeRequest) (*op.CodeResponseType, error) {
	userID := req.Username

	responseType := zitadeloidc.ResponseTypeCode
	if strings.TrimSpace(req.ResponseType) != "" {
		responseType = zitadeloidc.ResponseType(req.ResponseType)
	}
	responseMode := zitadeloidc.ResponseModeQuery
	if strings.TrimSpace(req.ResponseMode) != "" {
		responseMode = zitadeloidc.ResponseMode(req.ResponseMode)
	}

	authReq := &zitadeloidc.AuthRequest{
		ClientID:            strings.TrimSpace(req.ClientID),
		RedirectURI:         strings.TrimSpace(req.RedirectURI),
		Scopes:              strings.Fields(req.Scope),
		State:               req.State,
		Nonce:               req.Nonce,
		ResponseType:        responseType,
		ResponseMode:        responseMode,
		Prompt:              strings.Fields(req.Prompt),
		CodeChallenge:       strings.TrimSpace(req.CodeChallenge),
		CodeChallengeMethod: zitadeloidc.CodeChallengeMethod(strings.TrimSpace(req.CodeChallengeMethod)),
	}

	client, ok := s.findClient(authReq.ClientID)
	if !ok {
		return nil, errors.New("client not found")
	}
	if _, err := op.ValidateAuthRequestClient(ctx, authReq, s.newOIDCClient(client), s.provider.IDTokenHintVerifier(ctx)); err != nil {
		return nil, err
	}

	storedReq, err := s.CreateAuthRequest(ctx, authReq, userID)
	if err != nil {
		return nil, err
	}
	return op.BuildAuthResponseCodeResponsePayload(ctx, storedReq, s.provider)
}

func (s *Server) BuildAuthorizationCallbackURL(ctx context.Context, id string) (string, error) {
	authReq, err := s.AuthRequestByID(ctx, id)
	if err != nil {
		return "", err
	}
	if !authReq.Done() {
		return "", errors.New("auth request not completed")
	}
	return op.BuildAuthResponseCallbackURL(ctx, authReq, s.provider)
}

func (s *Server) BuildAuthorizationCallbackURLWithRedirect(ctx context.Context, id, redirectURI string) (string, error) {
	authReq, err := s.AuthRequestByID(ctx, id)
	if err != nil {
		return "", err
	}
	if !authReq.Done() {
		return "", errors.New("auth request not completed")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return op.BuildAuthResponseCallbackURL(ctx, authReq, s.provider)
	}
	codeResponse, err := op.BuildAuthResponseCodeResponsePayload(ctx, authReq, s.provider)
	if err != nil {
		return "", err
	}
	return op.AuthResponseURL(strings.TrimSpace(redirectURI), authReq.GetResponseType(), authReq.GetResponseMode(), codeResponse, s.provider.Encoder())
}

func (s *Server) CompleteAuthRequest(id, username string) error {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	req, ok := s.authRequests[id]
	if !ok {
		return errors.New("授权请求不存在或已过期")
	}
	req.UserID = username
	req.done = true
	if req.authTime.IsZero() {
		req.authTime = time.Now()
	}
	return nil
}

func (s *Server) CallbackURL(ctx context.Context, id string) string {
	return s.legacy.AuthCallbackURL()(ctx, id)
}

func (s *Server) CheckUsernamePassword(_ context.Context, username, password, id string) error {
	return s.Login(context.Background(), id, username, password)
}

func (s *Server) authenticate(ctx context.Context, username, password string) (string, error) {
	if s.authenticateUser != nil {
		return s.authenticateUser(ctx, username, password)
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return "", errors.New("用户名和密码不能为空")
	}
	clientSDK := k8s.NewK8sClient()
	if _, err := clientSDK.Login2(username, password, true); err != nil {
		return "", errors.New("用户名或密码错误")
	}
	return username, nil
}

func (s *Server) CreateAuthRequest(_ context.Context, authReq *zitadeloidc.AuthRequest, userID string) (op.AuthRequest, error) {
	if len(authReq.Prompt) == 1 && authReq.Prompt[0] == zitadeloidc.PromptNone && userID == "" {
		return nil, zitadeloidc.ErrLoginRequired()
	}
	req := &authRequest{
		ID:            uuid.NewString(),
		CreationDate:  time.Now(),
		ApplicationID: authReq.ClientID,
		CallbackURI:   authReq.RedirectURI,
		TransferState: authReq.State,
		Prompt:        append([]string{}, authReq.Prompt...),
		UserID:        userID,
		Scopes:        append([]string{}, authReq.Scopes...),
		ResponseType:  authReq.ResponseType,
		ResponseMode:  authReq.ResponseMode,
		Nonce:         authReq.Nonce,
		done:          userID != "",
	}
	if authReq.CodeChallenge != "" {
		req.CodeChallenge = &zitadeloidc.CodeChallenge{
			Challenge: authReq.CodeChallenge,
			Method:    authReq.CodeChallengeMethod,
		}
	}
	if req.done {
		req.authTime = time.Now()
	}
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	s.pruneAuthRequestsLocked()
	s.authRequests[req.ID] = req
	return req, nil
}

func (s *Server) AuthRequestByID(_ context.Context, id string) (op.AuthRequest, error) {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	req, ok := s.authRequests[id]
	if !ok {
		return nil, errors.New("request not found")
	}
	if req.CreationDate.Add(s.config.CodeTTL).Before(time.Now()) {
		delete(s.authRequests, id)
		return nil, errors.New("request expired")
	}
	return req, nil
}

func (s *Server) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	s.authReqMu.Lock()
	id, ok := s.authCodes[code]
	s.authReqMu.Unlock()
	if !ok {
		return nil, errors.New("code invalid or expired")
	}
	return s.AuthRequestByID(ctx, id)
}

func (s *Server) SaveAuthCode(_ context.Context, id string, code string) error {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	s.authCodes[code] = id
	return nil
}

func (s *Server) DeleteAuthRequest(_ context.Context, id string) error {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	delete(s.authRequests, id)
	for code, reqID := range s.authCodes {
		if reqID == id {
			delete(s.authCodes, code)
		}
	}
	return nil
}

func (r *authRequest) GetID() string  { return r.ID }
func (r *authRequest) GetACR() string { return "" }
func (r *authRequest) GetAMR() []string {
	if r.done {
		return []string{"pwd"}
	}
	return nil
}
func (r *authRequest) GetAudience() []string                        { return []string{r.ApplicationID} }
func (r *authRequest) GetAuthTime() time.Time                       { return r.authTime }
func (r *authRequest) GetClientID() string                          { return r.ApplicationID }
func (r *authRequest) GetCodeChallenge() *zitadeloidc.CodeChallenge { return r.CodeChallenge }
func (r *authRequest) GetNonce() string                             { return r.Nonce }
func (r *authRequest) GetRedirectURI() string                       { return r.CallbackURI }
func (r *authRequest) GetResponseType() zitadeloidc.ResponseType    { return r.ResponseType }
func (r *authRequest) GetResponseMode() zitadeloidc.ResponseMode    { return r.ResponseMode }
func (r *authRequest) GetScopes() []string                          { return r.Scopes }
func (r *authRequest) GetState() string                             { return r.TransferState }
func (r *authRequest) GetSubject() string                           { return r.UserID }
func (r *authRequest) Done() bool                                   { return r.done }
