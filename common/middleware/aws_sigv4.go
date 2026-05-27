package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	ranginemiddleware "github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	awsSigV4Algorithm  = "AWS4-HMAC-SHA256"
	emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

var (
	errMissingAuthorization = errors.New("missing authorization header")
	errInvalidAuthorization = errors.New("invalid authorization header")
	errInvalidCredential    = errors.New("invalid credential scope")
	errMissingAmzDate       = errors.New("missing x-amz-date header")
	errInvalidAmzDate       = errors.New("invalid x-amz-date header")
	errClockSkewExceeded    = errors.New("request time skew exceeds limit")
	errMissingSignedHeader  = errors.New("missing signed header")
	errSignatureMismatch    = errors.New("signature mismatch")
	errClientDisabled       = errors.New("api client disabled")
	errClientNotFound       = errors.New("api client not found")
)

type AwsSigV4 struct {
	ranginemiddleware.Abstract
	MaxSkew       time.Duration
	ResolveClient func(context.Context, string) (*apiclientv1alpha1.ApiClient, error)
}

type sigV4CredentialScope struct {
	AccessKeyID string
	Date        string
	Region      string
	Service     string
}

type sigV4AuthHeader struct {
	Credential    sigV4CredentialScope
	SignedHeaders []string
	Signature     string
}

func (m AwsSigV4) Process(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions {
		ctx.Next()
		return
	}

	authHeader, err := parseSigV4Authorization(ctx.Request.Header.Get("Authorization"))
	if err != nil {
		abortSigV4(ctx, err)
		return
	}

	client, err := m.resolveClient(ctx.Request.Context(), authHeader.Credential.AccessKeyID)
	if err != nil {
		abortSigV4(ctx, err)
		return
	}

	if !client.Spec.Enabled {
		abortSigV4(ctx, errClientDisabled)
		return
	}

	if err := verifySigV4Request(ctx.Request, client.Spec.ClientSecret, m.maxSkew(), time.Now().UTC()); err != nil {
		abortSigV4(ctx, err)
		return
	}

	ctx.Set("api_client_id", client.Spec.ClientID)
	ctx.Set("api_client_name", client.Spec.ClientName)
	ctx.Set("aws_sigv4_region", authHeader.Credential.Region)
	ctx.Set("aws_sigv4_service", authHeader.Credential.Service)
	ctx.Next()
}

func (m AwsSigV4) maxSkew() time.Duration {
	if m.MaxSkew <= 0 {
		return 5 * time.Minute
	}
	return m.MaxSkew
}

func (m AwsSigV4) resolveClient(ctx context.Context, clientID string) (*apiclientv1alpha1.ApiClient, error) {
	if m.ResolveClient != nil {
		return m.ResolveClient(ctx, clientID)
	}

	sdk := k8s.NewK8sClient()
	k8sClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}

	var list apiclientv1alpha1.ApiClientList
	if err := k8sClient.List(ctx, &list, sigclient.InNamespace(sdk.GetNamespace())); err != nil {
		return nil, err
	}

	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.ClientID == clientID {
			return item, nil
		}
	}

	return nil, errClientNotFound
}

func abortSigV4(ctx *gin.Context, err error) {
	message := "AWS SigV4 验证失败"
	switch {
	case errors.Is(err, errClientNotFound), errors.Is(err, errClientDisabled):
		message = "API Client 无效"
	case errors.Is(err, errClockSkewExceeded):
		message = "请求时间偏差过大"
	}

	ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code": http.StatusUnauthorized,
		"msg":  message,
	})
}

func verifySigV4Request(req *http.Request, secretKey string, maxSkew time.Duration, now time.Time) error {
	authHeader, err := parseSigV4Authorization(req.Header.Get("Authorization"))
	if err != nil {
		return err
	}

	requestTime, err := parseAmzDate(req.Header.Get("X-Amz-Date"))
	if err != nil {
		return err
	}
	if maxSkew > 0 {
		delta := now.Sub(requestTime)
		if delta < 0 {
			delta = -delta
		}
		if delta > maxSkew {
			return errClockSkewExceeded
		}
	}

	payloadHash, err := resolvePayloadHash(req)
	if err != nil {
		return err
	}

	canonicalRequest, err := buildCanonicalRequest(req, authHeader.SignedHeaders, payloadHash)
	if err != nil {
		return err
	}

	stringToSign := buildStringToSign(requestTime, authHeader.Credential, canonicalRequest)
	signingKey := deriveSigningKey(secretKey, authHeader.Credential.Date, authHeader.Credential.Region, authHeader.Credential.Service)
	expectedSignature := hmacHex(signingKey, stringToSign)

	if !hmac.Equal([]byte(strings.ToLower(expectedSignature)), []byte(strings.ToLower(authHeader.Signature))) {
		return errSignatureMismatch
	}

	return nil
}

func parseSigV4Authorization(value string) (*sigV4AuthHeader, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errMissingAuthorization
	}
	if !strings.HasPrefix(value, awsSigV4Algorithm+" ") {
		return nil, errInvalidAuthorization
	}

	parts := strings.Split(value[len(awsSigV4Algorithm)+1:], ",")
	fields := map[string]string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, errInvalidAuthorization
		}
		fields[kv[0]] = kv[1]
	}

	cred, ok := fields["Credential"]
	if !ok {
		return nil, errInvalidAuthorization
	}
	scopeParts := strings.Split(cred, "/")
	if len(scopeParts) != 5 || scopeParts[4] != "aws4_request" {
		return nil, errInvalidCredential
	}

	signedHeadersValue, ok := fields["SignedHeaders"]
	if !ok || signedHeadersValue == "" {
		return nil, errInvalidAuthorization
	}
	signature, ok := fields["Signature"]
	if !ok || signature == "" {
		return nil, errInvalidAuthorization
	}

	return &sigV4AuthHeader{
		Credential: sigV4CredentialScope{
			AccessKeyID: scopeParts[0],
			Date:        scopeParts[1],
			Region:      scopeParts[2],
			Service:     scopeParts[3],
		},
		SignedHeaders: strings.Split(signedHeadersValue, ";"),
		Signature:     signature,
	}, nil
}

func parseAmzDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, errMissingAmzDate
	}
	t, err := time.Parse("20060102T150405Z", value)
	if err != nil {
		return time.Time{}, errInvalidAmzDate
	}
	return t.UTC(), nil
}

func resolvePayloadHash(req *http.Request) (string, error) {
	if hash := strings.TrimSpace(req.Header.Get("X-Amz-Content-Sha256")); hash != "" {
		return hash, nil
	}
	if req.Body == nil {
		return emptyPayloadSHA256, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func buildCanonicalRequest(req *http.Request, signedHeaders []string, payloadHash string) (string, error) {
	canonicalHeaders, err := buildCanonicalHeaders(req, signedHeaders)
	if err != nil {
		return "", err
	}

	return strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQueryString(req.URL),
		canonicalHeaders,
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n"), nil
}

func buildCanonicalHeaders(req *http.Request, signedHeaders []string) (string, error) {
	lines := make([]string, 0, len(signedHeaders))
	for _, headerName := range signedHeaders {
		name := strings.ToLower(strings.TrimSpace(headerName))
		if name == "" {
			return "", errInvalidAuthorization
		}

		value, ok := headerValue(req, name)
		if !ok {
			return "", fmt.Errorf("%w: %s", errMissingSignedHeader, name)
		}
		lines = append(lines, name+":"+normalizeHeaderValue(value))
	}

	return strings.Join(lines, "\n") + "\n", nil
}

func headerValue(req *http.Request, name string) (string, bool) {
	if name == "host" {
		if req.Host != "" {
			return req.Host, true
		}
		if req.URL != nil && req.URL.Host != "" {
			return req.URL.Host, true
		}
		return "", false
	}

	values := req.Header.Values(name)
	if len(values) == 0 {
		return "", false
	}
	return strings.Join(values, ","), true
}

func normalizeHeaderValue(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func canonicalURI(u *url.URL) string {
	if u == nil {
		return "/"
	}
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return escapePath(path)
}

func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = awsEncode(segment, false)
	}
	escaped := strings.Join(segments, "/")
	if !strings.HasPrefix(escaped, "/") {
		return "/" + escaped
	}
	return escaped
}

func canonicalQueryString(u *url.URL) string {
	if u == nil || u.RawQuery == "" {
		return ""
	}

	values, _ := url.ParseQuery(u.RawQuery)
	type pair struct {
		key   string
		value string
	}
	pairs := make([]pair, 0, len(values))
	for key, items := range values {
		if len(items) == 0 {
			pairs = append(pairs, pair{key: awsEncode(key, false), value: ""})
			continue
		}
		for _, item := range items {
			pairs = append(pairs, pair{key: awsEncode(key, false), value: awsEncode(item, false)})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})

	var b strings.Builder
	for i, item := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(item.key)
		b.WriteByte('=')
		b.WriteString(item.value)
	}
	return b.String()
}

func buildStringToSign(requestTime time.Time, scope sigV4CredentialScope, canonicalRequest string) string {
	sum := sha256.Sum256([]byte(canonicalRequest))
	return strings.Join([]string{
		awsSigV4Algorithm,
		requestTime.UTC().Format("20060102T150405Z"),
		scope.Date + "/" + scope.Region + "/" + scope.Service + "/aws4_request",
		hex.EncodeToString(sum[:]),
	}, "\n")
}

func deriveSigningKey(secretKey, date, region, service string) []byte {
	kDate := hmacBytes([]byte("AWS4"+secretKey), date)
	kRegion := hmacBytes(kDate, region)
	kService := hmacBytes(kRegion, service)
	return hmacBytes(kService, "aws4_request")
}

func hmacBytes(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(data))
	return h.Sum(nil)
}

func hmacHex(key []byte, data string) string {
	return hex.EncodeToString(hmacBytes(key, data))
}

func awsEncode(value string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			b.WriteByte(ch)
			continue
		}
		if ch == '/' && !encodeSlash {
			b.WriteByte(ch)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", ch)
	}
	return b.String()
}
