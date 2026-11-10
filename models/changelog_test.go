package models

import (
	"testing"
	"time"

	"github.com/beego/beego/v2/client/orm"
	"github.com/mindoc-org/mindoc/conf"
)

// setDocTimes 直接改写文档的创建/修改时间（auto_now 字段无法通过插入指定）。
func setDocTimes(t *testing.T, docId int, createTime, modifyTime time.Time) {
	o := orm.NewOrm()
	if _, err := o.Raw("UPDATE md_documents SET create_time = ?, modify_time = ? WHERE document_id = ?",
		createTime, modifyTime, docId).Exec(); err != nil {
		t.Fatalf("更新文档时间失败: %v", err)
	}
}

func changeLogNames(changes []*DocumentChange) []string {
	names := make([]string, 0, len(changes))
	for _, change := range changes {
		names = append(names, change.DocumentName)
	}
	return names
}

func containsName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

// TestFindRecentChanges 验证变更记录查询：时间范围过滤、父标题合并、权限过滤、排序。
func TestFindRecentChanges(t *testing.T) {
	setupMigrateTestDB(t)

	o := orm.NewOrm()

	creator := &Member{Account: "creator", RealName: "创建者", Email: "creator@example.com"}
	if _, err := o.Insert(creator); err != nil {
		t.Fatalf("插入成员失败: %v", err)
	}
	modifier := &Member{Account: "modifier", RealName: "修改者", Email: "modifier@example.com"}
	if _, err := o.Insert(modifier); err != nil {
		t.Fatalf("插入成员失败: %v", err)
	}
	outsider := &Member{Account: "outsider", RealName: "外部成员", Email: "outsider@example.com"}
	if _, err := o.Insert(outsider); err != nil {
		t.Fatalf("插入成员失败: %v", err)
	}

	pub := insertBook(t, "cl-pub", "公共项目")
	priv := insertBook(t, "cl-priv", "私有项目")
	// 公开但设置访问密码的项目：匿名不应在变更记录里看到标题
	protected := insertBook(t, "cl-protected", "密码保护项目")
	protected.BookPassword = "secret"
	if _, err := o.Update(protected, "book_password"); err != nil {
		t.Fatalf("设置项目访问密码失败: %v", err)
	}
	priv.PrivatelyOwned = 1
	if _, err := o.Update(priv, "privately_owned"); err != nil {
		t.Fatalf("更新项目私有属性失败: %v", err)
	}
	if _, err := o.Insert(&Relationship{BookId: priv.BookId, MemberId: outsider.MemberId, RoleId: conf.BookObserver}); err != nil {
		t.Fatalf("插入项目关系失败: %v", err)
	}

	now := time.Now()
	recent := now.Add(-2 * 24 * time.Hour)
	old := now.AddDate(0, -3, 0)

	parent := insertDoc(t, pub.BookId, 0, 1, "cl-parent", "父文档")
	child := insertDoc(t, pub.BookId, parent.DocumentId, 1, "cl-child", "子文档")
	oldDoc := insertDoc(t, pub.BookId, 0, 2, "cl-old", "旧文档")
	privateDoc := insertDoc(t, priv.BookId, 0, 1, "cl-priv-doc", "私有文档")
	protectedDoc := insertDoc(t, protected.BookId, 0, 1, "cl-protected-doc", "密码保护文档")

	for _, doc := range []*Document{parent, child, privateDoc, protectedDoc} {
		doc.MemberId = creator.MemberId
		doc.ModifyAt = modifier.MemberId
		if _, err := o.Update(doc, "member_id", "modify_at"); err != nil {
			t.Fatalf("更新文档人员失败: %v", err)
		}
		setDocTimes(t, doc.DocumentId, recent, recent)
	}
	setDocTimes(t, oldDoc.DocumentId, old, old)

	since := now.AddDate(0, -1, 0)

	// 匿名访问：仅公开项目，旧文档被时间范围过滤，标题合并父文档
	changes, err := NewDocument().FindRecentChanges(since, 0, false, 100)
	if err != nil {
		t.Fatalf("查询变更记录失败: %v", err)
	}
	names := changeLogNames(changes)
	if len(names) != 2 {
		t.Fatalf("匿名访问应返回 2 条，实际 %d 条: %v", len(names), names)
	}
	if !containsName(names, "父文档 / 子文档") {
		t.Fatalf("子文档标题应合并父标题，实际: %v", names)
	}
	if containsName(names, "旧文档") {
		t.Fatalf("超出时间范围的文档不应返回: %v", names)
	}
	if containsName(names, "私有文档") {
		t.Fatalf("匿名访问不应看到私有项目文档: %v", names)
	}
	if containsName(names, "密码保护文档") {
		t.Fatalf("匿名访问不应看到密码保护项目的文档标题: %v", names)
	}
	for _, change := range changes {
		if change.ModifyBy != "修改者" || change.CreateBy != "创建者" {
			t.Fatalf("创建人/修改人解析异常: %+v", change)
		}
		if change.BookIdentify != "cl-pub" || change.BookName != "公共项目" {
			t.Fatalf("项目信息异常: %+v", change)
		}
	}

	// 项目成员：可以看到自己参与的私有项目
	changes, err = NewDocument().FindRecentChanges(since, outsider.MemberId, false, 100)
	if err != nil {
		t.Fatalf("查询变更记录失败: %v", err)
	}
	if !containsName(changeLogNames(changes), "私有文档") {
		t.Fatalf("项目成员应看到私有项目文档: %v", changeLogNames(changes))
	}
	if containsName(changeLogNames(changes), "密码保护文档") {
		t.Fatalf("非参与成员不应看到密码保护项目的文档: %v", changeLogNames(changes))
	}

	// 超级管理员：可以看到全部
	changes, err = NewDocument().FindRecentChanges(since, outsider.MemberId, true, 100)
	if err != nil {
		t.Fatalf("查询变更记录失败: %v", err)
	}
	if len(changes) != 4 {
		t.Fatalf("超级管理员应返回 4 条，实际 %d 条", len(changes))
	}

	// 不限时间范围：包含旧文档，且按修改时间倒序
	changes, err = NewDocument().FindRecentChanges(time.Time{}, outsider.MemberId, true, 100)
	if err != nil {
		t.Fatalf("查询变更记录失败: %v", err)
	}
	if !containsName(changeLogNames(changes), "旧文档") {
		t.Fatalf("不限时间范围时应包含旧文档: %v", changeLogNames(changes))
	}
	for i := 1; i < len(changes); i++ {
		if changes[i-1].ModifyTime.Before(changes[i].ModifyTime) {
			t.Fatalf("结果应按修改时间倒序: %v", changes)
		}
	}
}
