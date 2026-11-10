package models

import (
	"time"

	"github.com/beego/i18n"

	"fmt"
	"strconv"

	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/beego/beego/v2/client/orm"
	"github.com/beego/beego/v2/core/logs"
	"github.com/beego/beego/v2/server/web"
	"github.com/mindoc-org/mindoc/cache"
	"github.com/mindoc-org/mindoc/conf"
	"github.com/mindoc-org/mindoc/utils"
)

// Document struct.
type Document struct {
	DocumentId    int           `orm:"pk;auto;unique;column(document_id)" json:"doc_id"`
	DocumentName  string        `orm:"column(document_name);size(500);description(文档名称)" json:"doc_name"`
	Identify      string        `orm:"column(identify);size(100);index;null;default(null);description(唯一标识)" json:"identify"` // Identify 文档唯一标识
	BookId        int           `orm:"column(book_id);type(int);index;description(关联bools表主键)" json:"book_id"`
	ParentId      int           `orm:"column(parent_id);type(int);index;default(0);description(父级文档)" json:"parent_id"`
	OrderSort     int           `orm:"column(order_sort);default(0);type(int);index;description(排序从小到大排序)" json:"order_sort"`
	Markdown      string        `orm:"column(markdown);type(text);null;description(markdown内容)" json:"markdown"` // Markdown markdown格式文档.
	MarkdownTheme string        `orm:"column(markdown_theme);size(50);default(theme__light);description(markdown主题)" json:"markdown_theme"`
	Release       string        `orm:"column(release);type(text);null;description(文章内容)" json:"release"` // Release 发布后的Html格式内容.
	Content       string        `orm:"column(content);type(text);null;description(文章内容)" json:"content"` // Content 未发布的 Html 格式内容.
	CreateTime    time.Time     `orm:"column(create_time);type(datetime);auto_now_add;description(创建时间)" json:"create_time"`
	MemberId      int           `orm:"column(member_id);type(int);description(关系用户id)" json:"member_id"`
	ModifyTime    time.Time     `orm:"column(modify_time);type(datetime);auto_now;description(修改时间)" json:"modify_time"`
	ModifyAt      int           `orm:"column(modify_at);type(int);description(修改人id)" json:"-"`
	Version       int64         `orm:"column(version);type(bigint);description(版本，关联历史文档里的version)" json:"version"`
	IsOpen        int           `orm:"column(is_open);type(int);default(0);description(是否展开子目录 0：阅读时关闭节点 1：阅读时展开节点 2：空目录 单击时会展开下级节点)" json:"is_open"` //是否展开子目录：0 否/1 是 /2 空间节点，单击时展开下一级
	ViewCount     int           `orm:"column(view_count);type(int);description(浏览量)" json:"view_count"`
	AttachList    []*Attachment `orm:"-" json:"attach"`
	//i18n
	Lang string `orm:"-"`
}

// 多字段唯一键
func (item *Document) TableUnique() [][]string {
	return [][]string{{"book_id", "identify"}}
}

// TableName 获取对应数据库表名.
func (item *Document) TableName() string {
	return "documents"
}

// TableEngine 获取数据使用的引擎.
func (item *Document) TableEngine() string {
	return "INNODB"
}

func (item *Document) TableNameWithPrefix() string {
	return conf.GetDatabasePrefix() + item.TableName()
}

func NewDocument() *Document {
	return &Document{
		Version: time.Now().Unix(),
	}
}

//by yujian 仅更新文档的排序，不修改文档的修改时间. 更新排序 20241208 james
func (item *Document) UpateOrder(id int,OrderSort int,ParentId int) {
	o := orm.NewOrm()
	o.QueryTable(item.TableNameWithPrefix()).Filter("document_id", id).Update(orm.Params{
		"order_sort": OrderSort,"parent_id":ParentId,
	})
}

// 根据文档ID查询指定文档.
func (item *Document) Find(id int) (*Document, error) {
	if id <= 0 {
		return item, ErrInvalidParameter
	}

	o := orm.NewOrm()

	err := o.QueryTable(item.TableNameWithPrefix()).Filter("document_id", id).One(item)

	if err == orm.ErrNoRows {
		return item, ErrDataNotExist
	}

	return item, nil
}

// 插入和更新文档.
func (item *Document) InsertOrUpdate(cols ...string) error {
	o := orm.NewOrm()
	item.DocumentName = utils.StripTags(item.DocumentName)
	var err error
	if item.DocumentId > 0 {
		_, err = o.Update(item, cols...)
	} else {
		if item.Identify == "" {
			book := NewBook()
			identify := "docs"
			if err := o.QueryTable(book.TableNameWithPrefix()).Filter("book_id", item.BookId).One(book, "identify"); err == nil {
				identify = book.Identify
			}

			item.Identify = fmt.Sprintf("%s-%s", identify, strconv.FormatInt(time.Now().UnixNano(), 32))
		}

		if item.OrderSort == 0 {
			sort, _ := o.QueryTable(item.TableNameWithPrefix()).Filter("book_id", item.BookId).Filter("parent_id", item.ParentId).Count()
			item.OrderSort = int(sort) + 1
		}
		_, err = o.Insert(item)
		NewBook().ResetDocumentNumber(item.BookId)
	}
	if err != nil {
		return err
	}

	return nil
}

// 根据文档识别编号和项目id获取一篇文档
func (item *Document) FindByIdentityFirst(identify string, bookId int) (*Document, error) {
	o := orm.NewOrm()

	err := o.QueryTable(item.TableNameWithPrefix()).Filter("book_id", bookId).Filter("identify", identify).One(item)

	return item, err
}

// 递归删除一个文档.
func (item *Document) RecursiveDocument(docId int) error {

	o := orm.NewOrm()

	if doc, err := item.Find(docId); err == nil {
		// 删除文档的倒排索引
		index := NewContentReverseIndex()
		_ = index.DeleteByContentTypeAndContentId(1, docId)

		o.Delete(doc)
		NewDocumentHistory().Clear(doc.DocumentId)
	}
	var maps []orm.Params

	_, err := o.Raw("SELECT document_id FROM " + item.TableNameWithPrefix() + " WHERE parent_id=" + strconv.Itoa(docId)).Values(&maps)
	if err != nil {
		logs.Error("RecursiveDocument => ", err)
		return err
	}

	for _, param := range maps {
		if docId, ok := param["document_id"].(string); ok {
			id, _ := strconv.Atoi(docId)
			// 删除子文档的倒排索引
			index := NewContentReverseIndex()
			_ = index.DeleteByContentTypeAndContentId(1, id)

			o.QueryTable(item.TableNameWithPrefix()).Filter("document_id", id).Delete()
			item.RecursiveDocument(id)
		}
	}

	return nil
}

// MigrateToBook 将文档及其全部子文档迁移到另一个项目。
// 根文档将成为目标项目的顶级文档并排到最后（子文档保持原有相对顺序）；
// 与目标项目已有文档冲突的标识(identify)会自动追加时间后缀。返回迁移的文档数量。
func (item *Document) MigrateToBook(docId, sourceBookId, targetBookId int) (int, error) {
	if docId <= 0 || sourceBookId <= 0 || targetBookId <= 0 || sourceBookId == targetBookId {
		return 0, ErrInvalidParameter
	}

	o := orm.NewOrm()

	root, err := item.Find(docId)
	if err != nil {
		return 0, err
	}
	if root.BookId != sourceBookId {
		return 0, ErrInvalidParameter
	}

	// 读取源项目全部文档的 id/parent_id/identify，在内存中求子树
	var docs []*Document
	_, err = o.QueryTable(item.TableNameWithPrefix()).Filter("book_id", sourceBookId).
		All(&docs, "document_id", "parent_id", "identify")
	if err != nil {
		return 0, err
	}

	childrenMap := make(map[int][]int)
	identifyMap := make(map[int]string)
	for _, doc := range docs {
		childrenMap[doc.ParentId] = append(childrenMap[doc.ParentId], doc.DocumentId)
		identifyMap[doc.DocumentId] = doc.Identify
	}

	subtreeIds := make([]int, 0, 16)
	queue := []int{docId}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		subtreeIds = append(subtreeIds, id)
		queue = append(queue, childrenMap[id]...)
	}

	tx, err := o.Begin()
	if err != nil {
		return 0, err
	}

	// 目标项目顶级文档数量，用于把根文档排到最后
	rootCount, err := tx.QueryTable(item.TableNameWithPrefix()).
		Filter("book_id", targetBookId).Filter("parent_id", 0).Count()
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	for _, id := range subtreeIds {
		params := orm.Params{"book_id": targetBookId}
		// 标识在目标项目冲突时追加时间后缀，保证 (book_id, identify) 唯一键不被破坏
		if identify := identifyMap[id]; identify != "" {
			if tx.QueryTable(item.TableNameWithPrefix()).
				Filter("book_id", targetBookId).Filter("identify", identify).Exist() {
				params["identify"] = fmt.Sprintf("%s-%s", identify, strconv.FormatInt(time.Now().UnixNano(), 32))
			}
		}
		if id == docId {
			// 根文档成为目标项目的顶级文档并排到最后
			params["parent_id"] = 0
			params["order_sort"] = int(rootCount) + 1
		}
		if _, err := tx.QueryTable(item.TableNameWithPrefix()).Filter("document_id", id).Update(params); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		// 清理旧缓存（按 ID 与 按旧项目+标识两个键）
		cache.Delete("Document.Id." + strconv.Itoa(id))
		if identifyMap[id] != "" {
			cache.Delete(fmt.Sprintf("Document.BookId.%d.Identify.%s", sourceBookId, identifyMap[id]))
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	// 重置两个项目的文档数量统计
	NewBook().ResetDocumentNumber(sourceBookId)
	NewBook().ResetDocumentNumber(targetBookId)

	return len(subtreeIds), nil
}

// 将文档写入缓存
func (item *Document) PutToCache() {
	go func(m Document) {

		if m.Identify == "" {

			if err := cache.Put("Document.Id."+strconv.Itoa(m.DocumentId), m, time.Second*3600); err != nil {
				logs.Info("文档缓存失败:", m.DocumentId)
			}
		} else {
			if err := cache.Put(fmt.Sprintf("Document.BookId.%d.Identify.%s", m.BookId, m.Identify), m, time.Second*3600); err != nil {
				logs.Info("文档缓存失败:", m.DocumentId)
			}
		}

	}(*item)
}

// 清除缓存
func (item *Document) RemoveCache() {
	go func(m Document) {
		cache.Put("Document.Id."+strconv.Itoa(m.DocumentId), m, time.Second*3600)

		if m.Identify != "" {
			cache.Put(fmt.Sprintf("Document.BookId.%d.Identify.%s", m.BookId, m.Identify), m, time.Second*3600)
		}
	}(*item)
}

// 从缓存获取
func (item *Document) FromCacheById(id int) (*Document, error) {

	if err := cache.Get("Document.Id."+strconv.Itoa(id), &item); err == nil && item.DocumentId > 0 {
		logs.Info("从缓存中获取文档信息成功 ->", item.DocumentId)
		return item, nil
	}

	if item.DocumentId > 0 {
		item.PutToCache()
	}
	item, err := item.Find(id)

	if err == nil {
		item.PutToCache()
	}
	return item, err
}

// 根据文档标识从缓存中查询文档
func (item *Document) FromCacheByIdentify(identify string, bookId int) (*Document, error) {

	key := fmt.Sprintf("Document.BookId.%d.Identify.%s", bookId, identify)

	if err := cache.Get(key, item); err == nil && item.DocumentId > 0 {
		logs.Info("从缓存中获取文档信息成功 ->", key)
		return item, nil
	}

	defer func() {
		if item.DocumentId > 0 {
			item.PutToCache()
		}
	}()
	return item.FindByIdentityFirst(identify, bookId)
}

// 根据项目ID查询文档列表.
func (item *Document) FindListByBookId(bookId int) (docs []*Document, err error) {
	o := orm.NewOrm()

	_, err = o.QueryTable(item.TableNameWithPrefix()).Filter("book_id", bookId).OrderBy("order_sort").All(&docs)

	return
}

// 判断文章是否存在
func (item *Document) IsExist(documentId int) bool {
	o := orm.NewOrm()

	return o.QueryTable(item.TableNameWithPrefix()).Filter("document_id", documentId).Exist()
}

// 发布单篇文档
func (item *Document) ReleaseContent() error {

	item.Release = strings.TrimSpace(item.Content)

	err := item.Processor().InsertOrUpdate("release")

	if err != nil {
		logs.Error(fmt.Sprintf("发布失败 -> %+v", item), err)
		return err
	}
	//当文档发布后，需要清除已缓存的转换文档和文档缓存
	item.RemoveCache()

	if err := os.RemoveAll(filepath.Join(conf.WorkingDirectory, "uploads", "books", strconv.Itoa(item.BookId))); err != nil {
		logs.Error("删除已缓存的文档目录失败 -> ", filepath.Join(conf.WorkingDirectory, "uploads", "books", strconv.Itoa(item.BookId)))
		return err
	}

	// 刷新倒排索引
	go func(docId int, docName, release, markdown string) {
		content := release
		if content == "" {
			content = markdown
		}
		content = docName + "\n" + content
		content = utils.StripTags(content)
		if err := BuildIndexForDocument(docId, content); err != nil {
			logs.Error("error: 构建文档倒排索引失败 ->", docId, err)
		}
	}(item.DocumentId, item.DocumentName, item.Release, item.Markdown)

	return nil
}

// Processor 调用位置两处：
// 1. 项目发布和文档发布: 处理文档的外链，附件，底部编辑信息等;
// 2. 文档阅读：可以修复存在问题的文档，使其能正常显示附件下载和文档作者信息等。
func (item *Document) Processor() *Document {
	if item.Release != "" {
		item.Release = utils.SafetyProcessor(item.Release)
	} else {
		// Release内容为空，直接赋值文档标签，保证附件下载正常
		item.Release = "<div class=\"whole-article-wrap\"></div>"
	}

	// Next: 生成文档的一些附加信息
	if docQuery, err := goquery.NewDocumentFromReader(bytes.NewBufferString(item.Release)); err == nil {

		//处理附件
		if selector := docQuery.Find("div.attach-list").First(); selector.Size() <= 0 {
			//处理附件
			attachList, err := NewAttachment().FindListByDocumentId(item.DocumentId)
			if err == nil && len(attachList) > 0 {
				content := bytes.NewBufferString("<div class=\"attach-list\"><strong>" + i18n.Tr(item.Lang, "doc.attachment") + "</strong><ul>")
				for _, attach := range attachList {
					if strings.HasPrefix(attach.HttpPath, "/") {
						attach.HttpPath = strings.TrimSuffix(conf.BaseUrl, "/") + attach.HttpPath
					}
					li := fmt.Sprintf("<li><a href=\"%s\" target=\"_blank\" title=\"%s\">%s</a></li>", attach.HttpPath, attach.FileName, attach.FileName)

					content.WriteString(li)
				}
				content.WriteString("</ul></div>")
				if docQuery == nil {
					docQuery, err = goquery.NewDocumentFromReader(content)
					if err != nil {
						logs.Error("goquery->NewDocumentFromReader err:%+v", err)
					}
				} else {
					if selector := docQuery.Find("div.wiki-bottom").First(); selector.Size() > 0 {
						selector.BeforeHtml(content.String()) //This branch should be a compatible branch.
					} else if selector := docQuery.Find("div.markdown-article").First(); selector.Size() > 0 {
						selector.AppendHtml(content.String()) //The document produced by the editor of Markdown will have this tag.class.
					} else if selector := docQuery.Find("div.whole-article-wrap").First(); selector.Size() > 0 {
						selector.AppendHtml(content.String()) //All documents should have this tag.
					}
				}
			}
		}

		//处理了文档底部信息
		if selector := docQuery.Find("div.wiki-bottom").First(); selector.Size() <= 0 && item.MemberId > 0 {
			//处理文档结尾信息
			docCreator, err := NewMember().Find(item.MemberId, "real_name", "account")
			release := "<div class=\"wiki-bottom\">"

			release += i18n.Tr(item.Lang, "doc.ft_author")
			if err == nil && docCreator != nil {
				if docCreator.RealName != "" {
					release += docCreator.RealName
				} else {
					release += docCreator.Account
				}
			}
			release += " &nbsp;" + i18n.Tr(item.Lang, "doc.ft_create_time") + item.CreateTime.Local().Format("2006-01-02 15:04") + "<br>"

			if item.ModifyAt > 0 {
				docModify, err := NewMember().Find(item.ModifyAt, "real_name", "account")
				if err == nil {
					if docModify.RealName != "" {
						release += i18n.Tr(item.Lang, "doc.ft_last_editor") + docModify.RealName
					} else {
						release += i18n.Tr(item.Lang, "doc.ft_last_editor") + docModify.Account
					}
				}
			}
			release += " &nbsp;" + i18n.Tr(item.Lang, "doc.ft_update_time") + item.ModifyTime.Local().Format("2006-01-02 15:04") + "<br>"
			release += "</div>"

			if selector := docQuery.Find("div.markdown-article").First(); selector.Size() > 0 {
				selector.AppendHtml(release)
			} else if selector := docQuery.Find("div.whole-article-wrap").First(); selector.Size() > 0 {
				selector.AppendHtml(release)
			}
		}
		cdnimg, _ := web.AppConfig.String("cdnimg")

		docQuery.Find("img").Each(func(i int, selection *goquery.Selection) {

			if src, ok := selection.Attr("src"); ok {
				src = strings.TrimSpace(strings.ToLower(src))
				//过滤掉没有链接的图片标签
				if src == "" || strings.HasPrefix(src, "data:text/html") {
					selection.Remove()
					return
				}

				//设置图片为CDN地址
				if cdnimg != "" && strings.HasPrefix(src, "/uploads/") {
					selection.SetAttr("src", utils.JoinURI(cdnimg, src))
				}

			}
			selection.RemoveAttr("onerror").RemoveAttr("onload")
		})
		//过滤A标签的非法连接
		docQuery.Find("a").Each(func(i int, selection *goquery.Selection) {
			if val, exists := selection.Attr("href"); exists {
				if val == "" {
					selection.SetAttr("href", "#")
					return
				}
				val = strings.Replace(strings.ToLower(val), " ", "", -1)
				//移除危险脚本链接
				if strings.HasPrefix(val, "data:text/html") ||
					strings.HasPrefix(val, "vbscript:") ||
					strings.HasPrefix(val, "&#106;avascript:") ||
					strings.HasPrefix(val, "javascript:") {
					selection.SetAttr("href", "#")
				}
			}
			//移除所有 onerror 属性
			selection.RemoveAttr("onerror").RemoveAttr("onload").RemoveAttr("onclick")
		})

		docQuery.Find("script").Remove()
		docQuery.Find("link").Remove()
		docQuery.Find("vbscript").Remove()

		if html, err := docQuery.Html(); err == nil {
			item.Release = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(html), "<html><head></head><body>"), "</body></html>")
		}
	}

	return item
}

// 增加阅读次数
func (item *Document) IncrViewCount(id int) {
	o := orm.NewOrm()
	o.QueryTable(item.TableNameWithPrefix()).Filter("document_id", id).Update(orm.Params{
		"view_count": orm.ColValue(orm.ColAdd, 1),
	})
}
