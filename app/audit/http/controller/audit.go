package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	auditservice "github.com/w7panel/w7panel/common/service/audit"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Audit struct {
	controller.Abstract
}

func (self Audit) Status(ctx *gin.Context) {
	checkCtx, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Second)
	defer cancel()
	self.JsonResponseWithoutError(ctx, auditservice.CheckStatus(checkCtx))
}

func (self Audit) LoginLogs(ctx *gin.Context) {
	params := parseQueryParams(ctx)
	current := auditservice.CurrentUser(ctx)
	result, err := auditservice.QueryLoginLogs(ctx.Request.Context(), params, current)
	if err != nil {
		self.JsonResponseWithError(ctx, err, 500)
		return
	}
	self.JsonResponseWithoutError(ctx, result)
}

func (self Audit) OperationLogs(ctx *gin.Context) {
	params := parseQueryParams(ctx)
	current := auditservice.CurrentUser(ctx)
	result, err := auditservice.QueryOperationLogs(ctx.Request.Context(), params, current)
	if err != nil {
		self.JsonResponseWithError(ctx, err, 500)
		return
	}
	self.JsonResponseWithoutError(ctx, result)
}

func parseQueryParams(ctx *gin.Context) auditservice.QueryParams {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("pageSize", "20"))
	return auditservice.QueryParams{
		Page:      page,
		PageSize:  pageSize,
		Username:  ctx.Query("username"),
		Tenant:    ctx.Query("tenant"),
		Success:   ctx.Query("success"),
		Method:    ctx.Query("method"),
		Path:      ctx.Query("path"),
		StartTime: ctx.Query("startTime"),
		EndTime:   ctx.Query("endTime"),
	}
}
