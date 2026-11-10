package models

import (
	"time"

	"github.com/beego/beego/v2/client/orm"
)

// DocumentChange 文档变更记录（变更日志页面使用）。
type DocumentChange struct {
	DocumentId   int       `json:"doc_id"`
	DocumentName string    `json:"doc_name"`
	BookName     string    `json:"book_name"`
	BookIdentify string    `json:"book_identify"`
	DocIdentify  string    `json:"doc_identify"`
	ModifyTime   time.Time `json:"modify_time"`
	CreateTime   time.Time `json:"create_time"`
	ModifyBy     string    `json:"modify_by"`
	CreateBy     string    `json:"create_by"`
}

// documentChangeRow 变更记录查询的原始行。
type documentChangeRow struct {
	DocumentId   int       `orm:"column(document_id)"`
	DocumentName string    `orm:"column(document_name)"`
	DocIdentify  string    `orm:"column(doc_identify)"`
	BookName     string    `orm:"column(book_name)"`
	BookIdentify string    `orm:"column(book_identify)"`
	ModifyTime   time.Time `orm:"column(modify_time)"`
	CreateTime   time.Time `orm:"column(create_time)"`
	ModifyBy     string    `orm:"column(modify_by)"`
	CreateBy     string    `orm:"column(create_by)"`
	ParentName   string    `orm:"column(parent_name)"`
}

// FindRecentChanges 查询指定时间之后修改过的文档（按修改时间倒序）。
// 标题合并父文档标题（父 / 子）；memberId <= 0 表示匿名访问，只能看到公开项目；
// 非超级管理员额外可见自己参与的项目（直接关系或团队成员身份）。
func (item *Document) FindRecentChanges(since time.Time, memberId int, isAdministrator bool, limit int) ([]*DocumentChange, error) {
	o := orm.NewOrm()

	if limit <= 0 {
		limit = 500
	}

	docTable := item.TableNameWithPrefix()
	bookTable := NewBook().TableNameWithPrefix()
	memberTable := NewMember().TableNameWithPrefix()

	args := make([]interface{}, 0, 4)

	where := "1 = 1"
	if !since.IsZero() {
		where += " AND d.modify_time > ?"
		args = append(args, since)
	}
	if !isAdministrator {
		// 公开且无访问密码的项目匿名可见；密码保护的项目仅参与成员可见（含团队成员身份）
		where += " AND ((b.privately_owned = 0 AND (b.book_password = '' OR b.book_password IS NULL))"
		if memberId > 0 {
			where += " OR b.book_id IN (SELECT r.book_id FROM " + NewRelationship().TableNameWithPrefix() +
				" r WHERE r.member_id = ?)" +
				" OR b.book_id IN (SELECT tr.book_id FROM " + NewTeamRelationship().TableNameWithPrefix() +
				" tr INNER JOIN " + NewTeamMember().TableNameWithPrefix() +
				" tm ON tm.team_id = tr.team_id AND tm.member_id = ?)"
			args = append(args, memberId, memberId)
		}
		where += ")"
	}

	sql := "SELECT d.document_id, d.document_name, d.identify AS doc_identify," +
		" b.book_name, b.identify AS book_identify, d.modify_time, d.create_time," +
		" COALESCE(NULLIF(mu.real_name, ''), mu.account, '') AS modify_by," +
		" COALESCE(NULLIF(cu.real_name, ''), cu.account, '') AS create_by," +
		" COALESCE(p.document_name, '') AS parent_name" +
		" FROM " + docTable + " d" +
		" INNER JOIN " + bookTable + " b ON b.book_id = d.book_id" +
		" LEFT JOIN " + memberTable + " cu ON cu.member_id = d.member_id" +
		" LEFT JOIN " + memberTable + " mu ON mu.member_id = d.modify_at" +
		" LEFT JOIN " + docTable + " p ON p.document_id = d.parent_id" +
		" WHERE " + where +
		" ORDER BY d.modify_time DESC LIMIT ?"

	args = append(args, limit)

	var rows []documentChangeRow
	if _, err := o.Raw(sql, args...).QueryRows(&rows); err != nil {
		return nil, err
	}

	changes := make([]*DocumentChange, 0, len(rows))
	for _, row := range rows {
		name := row.DocumentName
		if row.ParentName != "" {
			name = row.ParentName + " / " + row.DocumentName
		}
		changes = append(changes, &DocumentChange{
			DocumentId:   row.DocumentId,
			DocumentName: name,
			BookName:     row.BookName,
			BookIdentify: row.BookIdentify,
			DocIdentify:  row.DocIdentify,
			ModifyTime:   row.ModifyTime,
			CreateTime:   row.CreateTime,
			ModifyBy:     row.ModifyBy,
			CreateBy:     row.CreateBy,
		})
	}

	return changes, nil
}
