package collab

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/beego/beego/v2/core/logs"
	"github.com/gorilla/websocket"
	"github.com/mindoc-org/mindoc/conf"
	"github.com/mindoc-org/mindoc/models"
)

const (
	writeWait      = 10 * time.Second    // 单次写超时
	pongWait       = 60 * time.Second    // 等待 pong 超时
	pingPeriod     = 54 * time.Second    // ping 周期（须小于 pongWait）
	maxMessageSize = 32 << 20            // 单帧消息上限（大文档全量快照）
	sendQueueSize  = 256                 // 每连接发送队列长度
)

// Server 协同编辑 WebSocket 服务，挂载在 /collab/* 路径下。
type Server struct {
	hub      *Hub
	upgrader websocket.Upgrader
	// docBelongsToBook 校验文档是否属于指定项目（生产环境查数据库，测试可注入）
	docBelongsToBook func(docId, bookId int) bool
}

// NewServer 创建协同服务。
func NewServer() *Server {
	s := &Server{
		hub: NewHub(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// 凭证采用短时 HMAC token 鉴权，不依赖 Origin
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	s.docBelongsToBook = func(docId, bookId int) bool {
		doc, err := models.NewDocument().Find(docId)
		return err == nil && doc != nil && doc.BookId == bookId
	}
	return s
}

// ServeHTTP 处理 /collab/{identify}/{docId}?token=... 的 WebSocket 升级请求。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 路径解析：/collab/{identify}/{docId}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "collab" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	identify := parts[1]
	docId, err := strconv.Atoi(parts[2])
	if err != nil || docId <= 0 {
		http.Error(w, "bad doc id", http.StatusBadRequest)
		return
	}

	// 校验凭证
	claims, err := ParseToken(r.URL.Query().Get("token"), []byte(conf.GetAppKey()))
	if err != nil {
		logs.Info("collab: token 校验失败 -> ", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if claims.Identify != identify || claims.DocID != docId {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// 复核文档归属（防止 token 签发后文档被移动）
	if s.docBelongsToBook == nil || !s.docBelongsToBook(docId, claims.BookID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logs.Error("collab: WebSocket 升级失败 -> ", err)
		return
	}

	conn := &Conn{
		send:   make(chan []byte, sendQueueSize),
		claims: claims,
	}
	roomName := fmt.Sprintf("book_%d_doc_%d", claims.BookID, claims.DocID)

	isFirst := s.hub.join(roomName, conn)
	logs.Informational("collab: 连接建立 room=%s member=%d first=%v", roomName, claims.MemberID, isFirst)

	if isFirst {
		// 告知该客户端为引导者：负责把数据库中的 markdown 写入 Y.Text
		conn.trySend([]byte{byte(msgBootstrapGrant)})
	}

	go s.writePump(conn, ws)
	s.readPump(conn, ws)

	s.hub.leave(conn)
	ws.Close()
	logs.Informational("collab: 连接关闭 room=%s member=%d", roomName, claims.MemberID)
}

// readPump 读取客户端消息并路由到房间，处理 pong 与读超时。
func (s *Server) readPump(c *Conn, ws *websocket.Conn) {
	ws.SetReadLimit(maxMessageSize)
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		_ = ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		msgType, data, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				logs.Info("collab: 连接异常断开 member=%d -> %v", c.claims.MemberID, err)
			}
			return
		}
		if msgType != websocket.BinaryMessage {
			continue // y-websocket 协议只使用二进制帧
		}
		if room := c.room; room != nil {
			room.handleMessage(c, data)
		}
	}
}

// writePump 将发送队列中的消息写给客户端，并定期 ping 保活。
func (s *Server) writePump(c *Conn, ws *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.sendMu.Lock()
			err := ws.WriteMessage(websocket.BinaryMessage, msg)
			c.sendMu.Unlock()
			if err != nil {
				return
			}
		case <-ticker.C:
			c.sendMu.Lock()
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			err := ws.WriteMessage(websocket.PingMessage, nil)
			c.sendMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}
