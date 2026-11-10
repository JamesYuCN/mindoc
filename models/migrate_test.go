package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	beegocache "github.com/beego/beego/v2/client/cache"
	"github.com/beego/beego/v2/client/orm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mindoc-org/mindoc/cache"
)

var migrateTestDBReady = false

// setupMigrateTestDB 初始化 sqlite 测试库（仅注册文档迁移用到的表与内存缓存）。
func setupMigrateTestDB(t *testing.T) {
	if migrateTestDBReady {
		return
	}

	cache.Init(beegocache.NewMemoryCache())

	dbFile := filepath.Join(os.TempDir(), "mindoc_migrate_test.db")
	_ = os.Remove(dbFile)

	if err := orm.RegisterDataBase("default", "sqlite3", dbFile); err != nil {
		t.Fatalf("注册测试数据库失败: %v", err)
	}

	orm.RegisterModelWithPrefix("md_",
		new(Member),
		new(Book),
		new(Relationship),
		new(Document),
		new(DocumentHistory),
		new(Team),
		new(TeamMember),
		new(TeamRelationship),
	)

	if err := orm.RunSyncdb("default", false, false); err != nil {
		t.Fatalf("同步表结构失败: %v", err)
	}

	migrateTestDBReady = true
}

func insertBook(t *testing.T, identify, name string) *Book {
	o := orm.NewOrm()
	book := &Book{BookName: name, Identify: identify}
	if _, err := o.Insert(book); err != nil {
		t.Fatalf("插入项目失败: %v", err)
	}
	return book
}

func insertDoc(t *testing.T, bookId, parentId, orderSort int, identify, name string) *Document {
	o := orm.NewOrm()
	doc := &Document{
		BookId:       bookId,
		ParentId:     parentId,
		OrderSort:    orderSort,
		Identify:     identify,
		DocumentName: name,
		Markdown:     "# " + name,
	}
	if _, err := o.Insert(doc); err != nil {
		t.Fatalf("插入文档失败: %v", err)
	}
	return doc
}

func getDoc(t *testing.T, id int) *Document {
	doc, err := NewDocument().Find(id)
	if err != nil {
		t.Fatalf("查询文档 %d 失败: %v", id, err)
	}
	return doc
}

// TestMigrateToBook 验证文档跨项目迁移：子树整体迁移、根文档排到最后、
// 标识冲突自动加后缀、非子树文档不受影响、项目文档数量统计重置。
func TestMigrateToBook(t *testing.T) {
	setupMigrateTestDB(t)

	src := insertBook(t, "src-book", "源项目")
	dst := insertBook(t, "dst-book", "目标项目")

	// 源项目：root -> child -> grandchild，另有无关文档 other
	root := insertDoc(t, src.BookId, 0, 1, "root-ident", "根文档")
	child := insertDoc(t, src.BookId, root.DocumentId, 1, "child-ident", "子文档")
	grandchild := insertDoc(t, src.BookId, child.DocumentId, 1, "grandchild-ident", "孙文档")
	other := insertDoc(t, src.BookId, 0, 2, "other-ident", "无关文档")

	// 目标项目：已有一个与 root 冲突的标识 + 两个顶级文档
	insertDoc(t, dst.BookId, 0, 1, "root-ident", "目标已有同名标识文档")
	insertDoc(t, dst.BookId, 0, 2, "y-ident", "目标文档Y")

	count, err := NewDocument().MigrateToBook(root.DocumentId, src.BookId, dst.BookId)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if count != 3 {
		t.Fatalf("迁移数量期望 3，实际 %d", count)
	}

	// 根文档：迁到目标项目，成为顶级文档，排序在最后（2 个顶级文档之后）
	newRoot := getDoc(t, root.DocumentId)
	if newRoot.BookId != dst.BookId {
		t.Fatalf("根文档 book_id 期望 %d，实际 %d", dst.BookId, newRoot.BookId)
	}
	if newRoot.ParentId != 0 {
		t.Fatalf("根文档 parent_id 期望 0，实际 %d", newRoot.ParentId)
	}
	if newRoot.OrderSort != 3 {
		t.Fatalf("根文档 order_sort 期望 3，实际 %d", newRoot.OrderSort)
	}
	// 标识与目标项目冲突，应自动追加后缀
	if newRoot.Identify == "root-ident" || !strings.HasPrefix(newRoot.Identify, "root-ident-") {
		t.Fatalf("根文档 identify 冲突后应加后缀，实际 %q", newRoot.Identify)
	}

	// 子孙文档：跟随迁移，父子关系与相对顺序保持不变
	newChild := getDoc(t, child.DocumentId)
	if newChild.BookId != dst.BookId || newChild.ParentId != root.DocumentId || newChild.OrderSort != 1 {
		t.Fatalf("子文档迁移结果异常: %+v", newChild)
	}
	newGrandchild := getDoc(t, grandchild.DocumentId)
	if newGrandchild.BookId != dst.BookId || newGrandchild.ParentId != child.DocumentId {
		t.Fatalf("孙文档迁移结果异常: %+v", newGrandchild)
	}
	if newGrandchild.Identify != "grandchild-ident" {
		t.Fatalf("无冲突的标识不应改变，实际 %q", newGrandchild.Identify)
	}

	// 无关文档不受影响
	newOther := getDoc(t, other.DocumentId)
	if newOther.BookId != src.BookId {
		t.Fatalf("无关文档不应被迁移，实际 book_id=%d", newOther.BookId)
	}

	// 项目文档数量统计
	srcBook, err := NewBook().Find(src.BookId)
	if err != nil {
		t.Fatalf("查询源项目失败: %v", err)
	}
	if srcBook.DocCount != 1 {
		t.Fatalf("源项目文档数量期望 1，实际 %d", srcBook.DocCount)
	}
	dstBook, err := NewBook().Find(dst.BookId)
	if err != nil {
		t.Fatalf("查询目标项目失败: %v", err)
	}
	if dstBook.DocCount != 5 {
		t.Fatalf("目标项目文档数量期望 5，实际 %d", dstBook.DocCount)
	}
}

// TestMigrateToBookInvalidParam 验证非法参数被拒绝。
func TestMigrateToBookInvalidParam(t *testing.T) {
	setupMigrateTestDB(t)

	src := insertBook(t, "src2-book", "源项目2")
	dst := insertBook(t, "dst2-book", "目标项目2")
	doc := insertDoc(t, src.BookId, 0, 1, "d-ident", "文档")

	// 目标项目与源项目相同
	if _, err := NewDocument().MigrateToBook(doc.DocumentId, src.BookId, src.BookId); err == nil {
		t.Fatal("迁移到相同项目应当报错")
	}
	// 文档不属于指定的源项目
	if _, err := NewDocument().MigrateToBook(doc.DocumentId, dst.BookId, dst.BookId); err == nil {
		t.Fatal("文档不属于源项目应当报错")
	}
	// 不存在的文档
	if _, err := NewDocument().MigrateToBook(99999, src.BookId, dst.BookId); err == nil {
		t.Fatal("不存在的文档应当报错")
	}
}
