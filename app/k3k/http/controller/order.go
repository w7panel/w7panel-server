package controller

import (
	"github.com/gin-gonic/gin"
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
	self.JsonSuccessResponse(http)
	return
}
