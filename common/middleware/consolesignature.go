package middleware

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/console"
	ranginemiddleware "github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

type ConsoleSignature struct {
	ranginemiddleware.Abstract
}

func (self ConsoleSignature) Process(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions {
		ctx.Next()
		return
	}

	licenseClient, err := console.NewDefaultLicenseClient()
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code": http.StatusUnauthorized,
			"msg":  "签名校验失败",
		})
		return
	}

	license, err := licenseClient.GetLicense()
	if err != nil || license == nil || license.AppId == "" || license.AppSecret == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code": http.StatusUnauthorized,
			"msg":  "签名校验失败",
		})
		return
	}

	ok, err := verifyConsoleRequestSignature(ctx.Request, license.AppId, license.AppSecret)
	if err != nil || !ok {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code": http.StatusUnauthorized,
			"msg":  "签名错误",
		})
		return
	}

	ctx.Next()
}

func verifyConsoleRequestSignature(req *http.Request, appID, appSecret string) (bool, error) {
	contentType := req.Header.Get("Content-Type")
	isJSON := strings.Contains(contentType, "application/json")

	if req.Method == http.MethodPost || req.Method == http.MethodPut {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return false, err
		}
		req.Body = io.NopCloser(bytes.NewBuffer(body))

		if isJSON && len(body) > 0 {
			return verifyConsoleJSONBodySignature(body, appID, appSecret)
		}

		values, err := url.ParseQuery(string(body))
		if err != nil {
			return false, err
		}
		return verifyConsoleValuesSignature(values, appID, appSecret)
	}

	return verifyConsoleValuesSignature(req.URL.Query(), appID, appSecret)
}

func verifyConsoleJSONBodySignature(body []byte, appID, appSecret string) (bool, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return false, err
	}

	sign, ok := data["sign"].(string)
	if !ok || sign == "" {
		return false, nil
	}
	delete(data, "sign")

	requestAppID, ok := data["appid"].(string)
	if !ok || requestAppID == "" || requestAppID != appID {
		return false, nil
	}
	if !hasConsoleSignatureMetaJSON(data) {
		return false, nil
	}

	signBytes, err := json.Marshal(data)
	if err != nil {
		return false, err
	}
	expected := md5.Sum([]byte(string(signBytes) + appSecret))
	return strings.EqualFold(sign, hex.EncodeToString(expected[:])), nil
}

func hasConsoleSignatureMetaJSON(data map[string]interface{}) bool {
	if _, ok := data["timestamp"]; !ok {
		return false
	}
	nonce, ok := data["nonce"].(string)
	return ok && nonce != ""
}

func verifyConsoleValuesSignature(values url.Values, appID, appSecret string) (bool, error) {
	sign := values.Get("sign")
	if sign == "" {
		return false, nil
	}
	requestAppID := values.Get("appid")
	if requestAppID == "" || requestAppID != appID {
		return false, nil
	}
	if values.Get("timestamp") == "" || values.Get("nonce") == "" {
		return false, nil
	}

	var keys []string
	signStr := ""
	for key := range values {
		if key == "sign" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		signStr += fmt.Sprintf("%s=%s", key, url.QueryEscape(values.Get(key)))
		if i < len(keys)-1 {
			signStr += "&"
		}
	}
	expected := md5.Sum([]byte(signStr + appSecret))
	return strings.EqualFold(sign, hex.EncodeToString(expected[:])), nil
}
