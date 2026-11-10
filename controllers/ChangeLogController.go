package controllers

import (
	"strconv"
	"strings"
	"time"

	"github.com/beego/beego/v2/core/logs"
	"github.com/mindoc-org/mindoc/conf"
	"github.com/mindoc-org/mindoc/models"
)

// ChangeLogController 文档变更记录页面。
// 页面不含页头页脚，供 iframe 嵌入使用；支持通过 URL 参数指定时间范围：
// /changelog?range=last_week|last_month|last_3month|last_6month|last_year|all
type ChangeLogController struct {
	BaseController
}

// changeLogRange 时间范围选项。
type changeLogRange struct {
	Label string
	Since func(now time.Time) time.Time
}

var changeLogRanges = map[string]changeLogRange{
	"last_week":   {Label: "最近一周", Since: func(now time.Time) time.Time { return now.AddDate(0, 0, -7) }},
	"last_month":  {Label: "最近1个月", Since: func(now time.Time) time.Time { return now.AddDate(0, -1, 0) }},
	"last_3month": {Label: "最近3个月", Since: func(now time.Time) time.Time { return now.AddDate(0, -3, 0) }},
	"last_6month": {Label: "最近6个月", Since: func(now time.Time) time.Time { return now.AddDate(0, -6, 0) }},
	"last_year":   {Label: "最近1年", Since: func(now time.Time) time.Time { return now.AddDate(-1, 0, 0) }},
	"all":         {Label: "全部", Since: func(now time.Time) time.Time { return time.Time{} }},
}

// changeLogView 变更记录页面展示项。
type changeLogView struct {
	DocumentName   string
	BookName       string
	URL            string
	ModifyTimeText string
	CreateTimeText string
	ModifyBy       string
	CreateBy       string
}

// Index 渲染变更记录页面。
func (c *ChangeLogController) Index() {
	rangeKey := strings.TrimSpace(c.GetString("range"))
	if _, ok := changeLogRanges[rangeKey]; !ok {
		rangeKey = "last_month"
	}
	rangeOpt := changeLogRanges[rangeKey]

	limit, _ := c.GetInt("limit", 500)
	if limit <= 0 || limit > 2000 {
		limit = 500
	}

	memberId := 0
	isAdmin := false
	if c.Member != nil {
		memberId = c.Member.MemberId
		isAdmin = c.Member.IsAdministrator()
	}

	changes, err := models.NewDocument().FindRecentChanges(rangeOpt.Since(time.Now()), memberId, isAdmin, limit)
	if err != nil {
		logs.Error("查询文档变更记录失败 ->", err)
	}

	items := make([]*changeLogView, 0, len(changes))
	for _, change := range changes {
		docId := change.DocIdentify
		if docId == "" {
			docId = strconv.Itoa(change.DocumentId)
		}
		items = append(items, &changeLogView{
			DocumentName:   change.DocumentName,
			BookName:       change.BookName,
			URL:            conf.URLFor("DocumentController.Read", ":key", change.BookIdentify, ":id", docId),
			ModifyTimeText: change.ModifyTime.Local().Format("2006-01-02 15:04:05"),
			CreateTimeText: change.CreateTime.Local().Format("2006-01-02 15:04:05"),
			ModifyBy:       change.ModifyBy,
			CreateBy:       change.CreateBy,
		})
	}

	c.Data["Changes"] = items
	c.Data["RangeLabel"] = rangeOpt.Label
	c.TplName = "changelog.tpl"
}
