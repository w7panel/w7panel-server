package controller

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/coredns"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type DNS struct {
	controller.Abstract
}

func (d DNS) Zones(ctx *gin.Context) {
	zones, err := coredns.NewService().ListZones(ctx.Request.Context())
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, zones)
}

func (d DNS) Info(ctx *gin.Context) {
	info, err := coredns.NewService().Info(ctx.Request.Context())
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
	zone, err := coredns.NewService().CreateZone(ctx.Request.Context(), req.Domain)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, zone)
}

func (d DNS) DeleteZone(ctx *gin.Context) {
	if err := coredns.NewService().DeleteZone(ctx.Request.Context(), ctx.Param("domain")); err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonSuccessResponse(ctx)
}

func (d DNS) Records(ctx *gin.Context) {
	records, err := coredns.NewService().ListRecords(ctx.Request.Context(), ctx.Param("domain"))
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
	record, err := coredns.NewService().CreateRecord(ctx.Request.Context(), ctx.Param("domain"), record)
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
	record, err := coredns.NewService().UpdateRecord(ctx.Request.Context(), ctx.Param("domain"), id, record)
	if err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonResponseWithoutError(ctx, record)
}

func (d DNS) DeleteRecord(ctx *gin.Context) {
	if err := coredns.NewService().DeleteRecord(ctx.Request.Context(), ctx.Param("domain"), ctx.Param("id")); err != nil {
		d.JsonResponseWithServerError(ctx, err)
		return
	}
	d.JsonSuccessResponse(ctx)
}

func (d DNS) Server(ctx *gin.Context) {
	status, err := coredns.NewService().ServerStatus(ctx.Request.Context())
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
	status, err := coredns.NewService().SetServerEnabled(ctx.Request.Context(), req.Enabled)
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
