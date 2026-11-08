// Package collab 为 MinDoc 内置的多人协同编辑服务。
//
// 该包实现 y-websocket 协议的服务端中继：服务器不解析 CRDT 文档内容，
// 仅负责消息转发、更新日志缓存（供迟到者回放）与 awareness（远程光标）状态清理。
package collab

import (
	"errors"
)

// y-websocket 消息类型（消息体第一个 varuint）
// 参考 y-websocket/src/y-websocket.js: messageSync=0, messageAwareness=1, messageAuth=2, messageQueryAwareness=3
const (
	msgSync           = 0 // 同步消息（CRDT 文档）
	msgAwareness      = 1 // awareness 消息（远程光标/用户信息），载荷外层有 varuint8Array 长度前缀
	msgAuth           = 2 // 鉴权消息（本服务用 HMAC token，不使用）
	msgQueryAwareness = 3 // 查询 awareness（客户端不会发送，忽略）
	// msgBootstrapGrant 是服务端 -> 客户端的自定义控制消息：
	// 告知客户端它是当前房间的"引导者"，负责把数据库中的 markdown 写入 Y.Text。
	// y-websocket 客户端对未知消息类型会静默忽略，因此该消息只应被 MinDoc 自己的前端
	// 协同模块（collab.min.js）处理。
	msgBootstrapGrant = 101
)

// y-protocols sync 子类型（sync 消息内第二个 varuint）
const (
	syncStep1 = 0 // 客户端 -> 服务端：状态向量，请求差量
	syncStep2 = 1 // 差量更新
	syncUpdate = 2 // 增量更新
)

var errMalformedMessage = errors.New("collab: malformed y-websocket message")

// readVarUint 读取 lib0 风格的无符号 varint（LEB128，最多 53 位）。
func readVarUint(b []byte, pos *int) (uint64, error) {
	var num uint64
	for i := 0; ; i++ {
		if *pos >= len(b) {
			return 0, errMalformedMessage
		}
		c := b[*pos]
		*pos++
		if i == 6 && c > 0x07 {
			return 0, errMalformedMessage // 超出 53 位
		}
		num |= uint64(c&0x7f) << (7 * i)
		if c < 0x80 {
			return num, nil
		}
	}
}

// writeVarUint 写入 lib0 风格的无符号 varint。
func writeVarUint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

// readVarString 读取 lib0 风格的字符串（varuint 长度 + 字节内容），返回字节切片（不拷贝）。
func readVarString(b []byte, pos *int) ([]byte, error) {
	l, err := readVarUint(b, pos)
	if err != nil {
		return nil, err
	}
	if uint64(len(b)-*pos) < l {
		return nil, errMalformedMessage
	}
	s := b[*pos : *pos+int(l)]
	*pos += int(l)
	return s, nil
}

// writeVarString 写入 lib0 风格的字符串。
func writeVarString(dst []byte, s []byte) []byte {
	dst = writeVarUint(dst, uint64(len(s)))
	return append(dst, s...)
}

// parseSyncHeader 解析消息头部，返回 sync 子类型与载荷起始位置。
// 输入 b 应为完整的一帧二进制消息。
func parseSyncHeader(b []byte) (subtype uint64, payloadPos int, err error) {
	pos := 0
	msgType, err := readVarUint(b, &pos)
	if err != nil {
		return 0, 0, err
	}
	if msgType != msgSync {
		return 0, 0, errMalformedMessage
	}
	subtype, err = readVarUint(b, &pos)
	if err != nil {
		return 0, 0, err
	}
	return subtype, pos, nil
}

// awareClient 记录某个 WebSocket 连接携带的 Yjs 客户端标识与逻辑时钟，
// 用于连接断开时构造 awareness 移除消息（清理远程光标）。
type awareClient struct {
	client uint64
	clock  uint64
}

// parseAwareness 解析 awareness 消息中的客户端列表。
// 线格式（lib0 编码，与 y-websocket 客户端一致）：
//
//	varuint 消息类型(1)
//	varuint 载荷长度（varuint8Array 长度前缀）
//	载荷: varuint 客户端数量
//	      每个客户端: varuint clientID, varuint clock, varstring state(JSON)
//
// 解析失败时返回 nil（容忍协议细节差异，退化为不清理该连接的光标）。
func parseAwareness(b []byte) []awareClient {
	pos := 0
	msgType, err := readVarUint(b, &pos)
	if err != nil || msgType != msgAwareness {
		return nil
	}
	payload, err := readVarString(b, &pos)
	if err != nil {
		return nil
	}

	p := 0
	count, err := readVarUint(payload, &p)
	if err != nil || count == 0 || count > 1000 {
		return nil
	}
	clients := make([]awareClient, 0, count)
	for i := uint64(0); i < count; i++ {
		clientID, err := readVarUint(payload, &p)
		if err != nil {
			return nil
		}
		clock, err := readVarUint(payload, &p)
		if err != nil {
			return nil
		}
		if _, err := readVarString(payload, &p); err != nil {
			return nil
		}
		clients = append(clients, awareClient{client: clientID, clock: clock})
	}
	return clients
}

// buildAwarenessRemove 构造移除指定 Yjs 客户端 awareness 状态的消息（state=null, clock+1），
// 与 y-websocket 官方服务端断开清理逻辑一致。
// 线格式：varuint(1) + varuint8Array(载荷)，载荷内为各客户端的 null 状态。
func buildAwarenessRemove(clients []awareClient) []byte {
	payload := make([]byte, 0, 16*len(clients)+8)
	payload = writeVarUint(payload, uint64(len(clients)))
	for _, c := range clients {
		payload = writeVarUint(payload, c.client)
		payload = writeVarUint(payload, c.clock+1)
		payload = writeVarString(payload, []byte("null"))
	}

	msg := make([]byte, 0, len(payload)+8)
	msg = writeVarUint(msg, msgAwareness)
	msg = writeVarUint(msg, uint64(len(payload)))
	return append(msg, payload...)
}

// buildEmptySyncStep2 构造空载荷的 SyncStep2 消息。
// 房间日志为空时回放它，触发客户端的 synced 事件。
// 注意：空文档的 Yjs update 编码为 [0x00, 0x00]（2 字节），零字节载荷会导致客户端
// 解码报错 "Unexpected end of array"。
func buildEmptySyncStep2() []byte {
	return []byte{byte(msgSync), byte(syncStep2), 2, 0, 0}
}

// buildSyncStep2 用一段更新载荷构造 SyncStep2 消息。
// 注意：update 必须是带 varuint8Array 长度前缀的载荷（与 y-protocols writeUpdate 的
// 线格式一致），房间更新日志中缓存的正是这种格式。
func buildSyncStep2(update []byte) []byte {
	msg := make([]byte, 0, len(update)+2)
	msg = writeVarUint(msg, msgSync)
	msg = writeVarUint(msg, syncStep2)
	return append(msg, update...)
}
