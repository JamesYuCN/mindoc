package collab

import (
	"sync"
	"time"

	"github.com/beego/beego/v2/core/logs"
)

// DefaultTokenTTL 协同连接凭证有效期。页面在进入编辑/切换文档时获取新 token，
// 断线重连一般能在有效期内完成；过期后前端会重新请求 token。
const DefaultTokenTTL = time.Hour

// updateLogWarnThreshold 更新日志超过该大小时记录告警日志（仍继续追加以保证正确性）。
const updateLogWarnThreshold = 32 << 20 // 32MB

// Conn 表示一个 WebSocket 连接。
type Conn struct {
	send    chan []byte
	room    *Room
	claims  *Claims
	joined  time.Time
	sendMu  sync.Mutex // 保护底层 WebSocket 写操作（ping 与业务消息并发写）
	closed  bool
	closeMu sync.Mutex
}

// trySend 向客户端发送一条二进制消息，发送队列满或连接已关闭时丢弃并告警。
func (c *Conn) trySend(msg []byte) {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()
	if closed {
		return
	}
	select {
	case c.send <- msg:
	default:
		logs.Warn("collab: 连接发送队列已满，丢弃消息 room=%s member=%d", c.room.name, c.claims.MemberID)
	}
}

// close 标记连接已关闭，后续消息不再入队。
func (c *Conn) close() {
	c.closeMu.Lock()
	c.closed = true
	c.closeMu.Unlock()
}

// Room 表示一个协同编辑房间，对应一篇文档。
// 服务器不解析 CRDT 内容，仅缓存更新日志与 awareness 状态。
type Room struct {
	mu       sync.Mutex
	name     string
	conns    map[*Conn]struct{}
	updates  [][]byte             // Yjs 更新日志，迟到者全量回放（Yjs update 幂等）
	logBytes int                  // 日志总字节数
	aware    map[*Conn][][]byte   // 各连接最近发送的 awareness 原始消息缓存
	awareCl  map[*Conn][]awareClient // 各连接携带的 Yjs 客户端标识
	boot     *Conn                 // 当前引导者（负责写入初始 markdown）
}

// Hub 管理所有房间。
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

// NewHub 创建房间管理器。
func NewHub() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

// join 将连接加入房间，返回该连接是否为房间的引导者（房间此前无人）。
func (h *Hub) join(roomName string, c *Conn) bool {
	h.mu.Lock()
	room, ok := h.rooms[roomName]
	if !ok {
		room = &Room{
			name:    roomName,
			conns:   make(map[*Conn]struct{}),
			aware:   make(map[*Conn][][]byte),
			awareCl: make(map[*Conn][]awareClient),
		}
		h.rooms[roomName] = room
	}
	h.mu.Unlock()

	c.room = room
	c.joined = time.Now()

	room.mu.Lock()
	room.conns[c] = struct{}{}
	isFirst := room.boot == nil
	if isFirst {
		room.boot = c
	}
	// 给迟到者回放缓存的 awareness（其他人的光标/用户信息）
	var cached []([]byte)
	for other, msgs := range room.aware {
		if other != c {
			cached = append(cached, msgs...)
		}
	}
	room.mu.Unlock()

	for _, m := range cached {
		c.trySend(m)
	}
	return isFirst
}

// leave 将连接移出房间，清理 awareness 状态；房间为空时销毁。
func (h *Hub) leave(c *Conn) {
	room := c.room
	if room == nil {
		return
	}
	c.close()

	room.mu.Lock()
	delete(room.conns, c)
	clients := room.awareCl[c]
	delete(room.aware, c)
	delete(room.awareCl, c)

	// 引导者离开且房间仍有其他人：提升最早加入的连接为新引导者
	var promoted *Conn
	if room.boot == c {
		room.boot = nil
		for other := range room.conns {
			if promoted == nil || other.joined.Before(promoted.joined) {
				promoted = other
			}
		}
		room.boot = promoted
	}
	empty := len(room.conns) == 0
	room.mu.Unlock()

	// 广播移除离开者的远程光标
	if len(clients) > 0 {
		room.broadcast(nil, buildAwarenessRemove(clients))
	}

	// 通知新引导者
	if promoted != nil {
		promoted.trySend([]byte{byte(msgBootstrapGrant)})
	}

	if empty {
		h.mu.Lock()
		// 二次确认房间仍为空（期间可能有新连接加入）
		room.mu.Lock()
		stillEmpty := len(room.conns) == 0
		room.mu.Unlock()
		if stillEmpty {
			delete(h.rooms, room.name)
		}
		h.mu.Unlock()
	}
}

// handleMessage 处理来自客户端的一条二进制消息。
func (r *Room) handleMessage(c *Conn, data []byte) {
	pos := 0
	msgType, err := readVarUint(data, &pos)
	if err != nil {
		return
	}

	switch msgType {
	case msgSync:
		subtype, err := readVarUint(data, &pos)
		if err != nil {
			return
		}
		switch subtype {
		case syncStep1:
			// 客户端请求差量：回放全部更新日志（每条单独一条 SyncStep2）；
			// 日志为空时发送空 Step2 以触发客户端 synced 事件
			r.replayTo(c)
		case syncStep2, syncUpdate:
			// 服务器不主动发送 SyncStep1，正常不会收到 Step2；
			// 一旦收到（其他实现），按增量更新处理即可（幂等）
			r.appendUpdate(data[pos:])
			r.broadcast(c, data)
		}
	case msgAwareness:
		clients := parseAwareness(data)
		r.mu.Lock()
		r.aware[c] = [][]byte{data}
		if clients != nil {
			r.awareCl[c] = clients
		}
		r.mu.Unlock()
		r.broadcast(c, data)
	case msgQueryAwareness:
		// 客户端不会发送该消息，忽略
	default:
		// 未知消息类型忽略（保持与 y-websocket 客户端一致的容忍行为）
	}
}

// replayTo 向指定连接回放更新日志。
func (r *Room) replayTo(c *Conn) {
	r.mu.Lock()
	logs := make([][]byte, len(r.updates))
	copy(logs, r.updates)
	r.mu.Unlock()

	if len(logs) == 0 {
		c.trySend(buildEmptySyncStep2())
		return
	}
	for _, u := range logs {
		c.trySend(buildSyncStep2(u))
	}
}

// appendUpdate 追加一条更新到日志。
func (r *Room) appendUpdate(update []byte) {
	if len(update) == 0 {
		return
	}
	r.mu.Lock()
	r.updates = append(r.updates, update)
	r.logBytes += len(update)
	warn := r.logBytes > updateLogWarnThreshold
	name := r.name
	size := r.logBytes
	r.mu.Unlock()

	if warn {
		logs.Warn("collab: 房间更新日志较大 room=%s size=%d bytes", name, size)
	}
}

// broadcast 向房间内除 exclude 外的所有连接广播消息（exclude 为 nil 时广播给所有人）。
func (r *Room) broadcast(exclude *Conn, msg []byte) {
	r.mu.Lock()
	targets := make([]*Conn, 0, len(r.conns))
	for c := range r.conns {
		if c != exclude {
			targets = append(targets, c)
		}
	}
	r.mu.Unlock()

	for _, c := range targets {
		c.trySend(msg)
	}
}

// Online 返回房间当前连接数（调试/状态用）。
func (r *Room) Online() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.conns)
}
