package controller

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Order struct {
	controller.Abstract
}

// 授权购买
func (self Order) CreateLicenseOrder(http *gin.Context) {
	type ParamsValidate struct {
		ProductId string `form:"productId" validate:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	sdkClient, err := console.NewDefaultSdkClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	result, err := sdkClient.CreateDefaultProductOrder(params.ProductId)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, result)
	return
}

func (self Order) OrderNotify(http *gin.Context) {
	var params struct {
		K3kName string `json:"k3kName" form:"k3kName"`
		OrderSn string `json:"orderSn" form:"orderSn"`
	}
	if err := http.ShouldBind(&params); err != nil {
		self.JsonResponseWithServerError(http, fmt.Errorf("invalid order notification: %w", err))
		return
	}
	params.K3kName = strings.TrimSpace(params.K3kName)
	params.OrderSn = strings.TrimSpace(params.OrderSn)
	if params.K3kName == "" || params.OrderSn == "" {
		self.JsonResponseWithServerError(http, errors.New("k3kName and orderSn are required"))
		return
	}

	endpoint := os.Getenv("CKM_ORDER_NOTIFY_URL")
	if endpoint == "" {
		endpoint = "http://w7panel-ckm.default.svc:8001/ckm-api/v1/internal/order/notify"
	}
	resp, err := helper.RetryHttpClient().R().
		SetFormData(map[string]string{"k3kName": params.K3kName, "orderSn": params.OrderSn}).
		Post(endpoint)
	if err != nil {
		self.JsonResponseWithServerError(http, fmt.Errorf("forward order notification: %w", err))
		return
	}
	if resp.StatusCode() != 200 {
		self.JsonResponseWithServerError(http, fmt.Errorf("ckm order notification returned status %d", resp.StatusCode()))
		return
	}
	self.JsonSuccessResponse(http)
}
