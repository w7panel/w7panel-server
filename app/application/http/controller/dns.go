package controller

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/coredns"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type DNS struct {
	controller.Abstract
}

var errK3kDNSServerUnsupported = errors.New("private dns server is not supported in k3k cluster")

func (d DNS) Zones(ctx *gin.Context) {
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	zones, err := service.ListZones(ctx.Request.Context())
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, zones)
}

func (d DNS) Info(ctx *gin.Context) {
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	info, err := service.Info(ctx.Request.Context())
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, info)
}

func (d DNS) CreateZone(ctx *gin.Context) {
	type request struct {
		Domain string `json:"domain" binding:"required"`
	}
	req := request{}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	zone, err := service.CreateZone(ctx.Request.Context(), req.Domain)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, zone)
}

func (d DNS) DeleteZone(ctx *gin.Context) {
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	if err := service.DeleteZone(ctx.Request.Context(), ctx.Param("domain")); err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonSuccessResponse(ctx)
}

func (d DNS) Records(ctx *gin.Context) {
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	records, err := service.ListRecords(ctx.Request.Context(), ctx.Param("domain"))
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, records)
}

func (d DNS) CreateRecord(ctx *gin.Context) {
	record, ok := d.bindRecord(ctx)
	if !ok {
		return
	}
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	record, err = service.CreateRecord(ctx.Request.Context(), ctx.Param("domain"), record)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, record)
}

func (d DNS) UpdateRecord(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		d.JsonResponseWithServerError(ctx, errors.New("record id is required"))
		return
	}
	record, ok := d.bindRecord(ctx)
	if !ok {
		return
	}
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	record, err = service.UpdateRecord(ctx.Request.Context(), ctx.Param("domain"), id, record)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, record)
}

func (d DNS) DeleteRecord(ctx *gin.Context) {
	service, _, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	if err := service.DeleteRecord(ctx.Request.Context(), ctx.Param("domain"), ctx.Param("id")); err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonSuccessResponse(ctx)
}

func (d DNS) Server(ctx *gin.Context) {
	service, isK3k, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	if isK3k {
		d.JsonResponseWithoutError(ctx, k3kDNSServerStatus())
		return
	}
	status, err := service.ServerStatus(ctx.Request.Context())
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, status)
}

func (d DNS) UpdateServer(ctx *gin.Context) {
	type request struct {
		Enabled bool `json:"enabled"`
	}
	req := request{}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	service, isK3k, err := d.service(ctx)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	if isK3k {
		d.JsonResponseWithServerError(ctx, errK3kDNSServerUnsupported)
		return
	}
	status, err := service.SetServerEnabled(ctx.Request.Context(), req.Enabled)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, status)
}

func (d DNS) bindRecord(ctx *gin.Context) (coredns.Record, bool) {
	req := coredns.Record{}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return coredns.Record{}, false
	}
	return req, true
}

func (d DNS) service(ctx *gin.Context) (*coredns.Service, bool, error) {
	token := ctx.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	if !k8sToken.IsK3kCluster() {
		return coredns.NewService(), false, nil
	}
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		return nil, true, err
	}
	return coredns.NewServiceWithSdk(sdk), true, nil
}

func k3kDNSServerStatus() coredns.ServerStatus {
	return coredns.ServerStatus{ServiceName: coredns.PublicDNSServiceName}
}
