package controllers

import (
	"time"

	"github.com/beego/i18n"
	"github.com/mindoc-org/mindoc/collab"
	"github.com/mindoc-org/mindoc/conf"
	"github.com/mindoc-org/mindoc/models"
)

// CollabController 多人协同编辑辅助接口。
type CollabController struct {
	BaseController
}

// Token 签发协同编辑的 WebSocket 连接凭证。
// 凭证为短时 HMAC token（默认 1 小时），由内置的 /collab/* WebSocket 服务校验，
// 避免 WebSocket 升级请求中重复实现 beego 会话鉴权。
func (c *CollabController) Token() {
	c.Prepare()

	if !conf.GetEnableCollab() {
		c.JsonResult(6001, i18n.Tr(c.Lang, "message.failed"))
		return
	}

	if !c.isUserLoggedIn() {
		c.JsonResult(6002, i18n.Tr(c.Lang, "message.no_permission"))
		return
	}

	if c.Member.Role == conf.MemberReaderRole {
		c.JsonResult(6002, i18n.Tr(c.Lang, "message.no_permission"))
		return
	}

	identify := c.Ctx.Input.Param(":key")
	docId, err := c.GetInt("doc_id")
	if err != nil {
		docId = 0
	}
	if identify == "" || docId <= 0 {
		c.JsonResult(6004, i18n.Tr(c.Lang, "message.param_error"))
		return
	}

	bookId := 0

	// 如果是超级管理员，则不判断权限
	if c.Member.IsAdministrator() {
		book, err := models.NewBook().FindByFieldFirst("identify", identify)
		if err != nil || book == nil {
			c.JsonResult(6003, i18n.Tr(c.Lang, "message.item_not_exist_or_no_permit"))
			return
		}
		bookId = book.BookId
	} else {
		bookResult, err := models.NewBookResult().FindByIdentify(identify, c.Member.MemberId)
		if err != nil || bookResult.RoleId == conf.BookObserver {
			c.JsonResult(6003, i18n.Tr(c.Lang, "message.item_not_exist_or_no_permit"))
			return
		}
		bookId = bookResult.BookId
	}

	// 校验文档归属
	doc, err := models.NewDocument().Find(docId)
	if err != nil || doc == nil || doc.BookId != bookId {
		c.JsonResult(6004, i18n.Tr(c.Lang, "message.doc_not_exist"))
		return
	}

	claims := collab.Claims{
		MemberID: c.Member.MemberId,
		BookID:   bookId,
		DocID:    docId,
		Identify: identify,
		Expires:  time.Now().Add(collab.DefaultTokenTTL).Unix(),
	}

	token, err := collab.SignToken(claims, []byte(conf.GetAppKey()))
	if err != nil {
		c.JsonResult(6006, i18n.Tr(c.Lang, "message.failed"))
		return
	}

	c.JsonResult(0, "ok", map[string]interface{}{
		"token":      token,
		"expires_in": int(collab.DefaultTokenTTL.Seconds()),
	})
}
