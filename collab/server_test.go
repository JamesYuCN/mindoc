package collab

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mindoc-org/mindoc/conf"
)

// 使用真实 yjs 编码的协议字节（见 protocol_test.go 的 fixture 说明）
const (
	itUpdateHex     = "0002200101ae9df4820800040107636f6e74656e740c68656c6c6f206d696e646f6300"
	itAwarenessHex  = "015901ae9df4820802517b2275736572223a7b226e616d65223a22e5bca0e4b889222c22636f6c6f72223a2223333062636564227d2c22637572736f72223a7b22616e63686f72223a6e756c6c2c2268656164223a6e756c6c7d7d"
	itClientID      = 2153582254
)

type itClient struct {
	t  *testing.T
	ws *websocket.Conn
	ch chan []byte
}

// startReader 启动独立读协程（gorilla/websocket 读超时后连接即损坏，不能用于 drain）
func newItClient(t *testing.T, ws *websocket.Conn) *itClient {
	c := &itClient{t: t, ws: ws, ch: make(chan []byte, 64)}
	go func() {
		defer close(c.ch)
		for {
			msgType, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if msgType != websocket.BinaryMessage {
				continue
			}
			c.ch <- data
		}
	}()
	return c
}

func (c *itClient) send(b []byte) {
	c.t.Helper()
	if err := c.ws.WriteMessage(websocket.BinaryMessage, b); err != nil {
		c.t.Fatalf("send: %v", err)
	}
}

// recv 等待一条消息，超时返回 nil
func (c *itClient) recv(timeout time.Duration) []byte {
	c.t.Helper()
	select {
	case m, ok := <-c.ch:
		if !ok {
			return nil
		}
		return m
	case <-time.After(timeout):
		return nil
	}
}

// drain 收集当前所有 pending 消息，并额外等待 extra 确认无更多消息
func (c *itClient) drain(extra time.Duration) [][]byte {
	var msgs [][]byte
	for {
		select {
		case m, ok := <-c.ch:
			if !ok {
				return msgs
			}
			msgs = append(msgs, m)
			continue
		default:
		}
		select {
		case m, ok := <-c.ch:
			if !ok {
				return msgs
			}
			msgs = append(msgs, m)
		case <-time.After(extra):
			return msgs
		}
	}
}

func (c *itClient) hasMsg(msgs [][]byte, prefix string) bool {
	p, _ := hex.DecodeString(prefix)
	for _, m := range msgs {
		if bytes.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	s := NewServer()
	s.docBelongsToBook = func(docId, bookId int) bool { return true }
	ts := httptest.NewServer(http.HandlerFunc(s.ServeHTTP))
	t.Cleanup(ts.Close)
	return ts, s
}

func dialCollab(t *testing.T, ts *httptest.Server, claims Claims) *itClient {
	t.Helper()
	token, err := SignToken(claims, []byte(conf.GetAppKey()))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	url := "ws" + strings.TrimPrefix(ts.URL, "http") +
		"/collab/" + claims.Identify + "/" + itoa(claims.DocID) + "?token=" + token
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { ws.Close() })
	return newItClient(t, ws)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func TestCollabServerIntegration(t *testing.T) {
	ts, server := newTestServer(t)
	_ = server

	claims := Claims{MemberID: 1, BookID: 2, DocID: 3, Identify: "itbook"}

	// --- 无 token 拒绝 ---
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/collab/itbook/3"
	if _, resp, err := websocket.DefaultDialer.Dial(wsURL, nil); err == nil {
		t.Fatal("无 token 应当被拒绝")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("期望 401，得到 %v", resp)
	}

	// --- token 与文档不匹配拒绝 ---
	badClaims := claims
	badClaims.DocID = 999
	badToken, _ := SignToken(badClaims, []byte(conf.GetAppKey()))
	if _, resp, err := websocket.DefaultDialer.Dial(wsURL+"?token="+badToken, nil); err == nil {
		t.Fatal("token 不匹配应当被拒绝")
	} else if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("期望 403，得到 %v", resp)
	}

	// --- 客户端 A 连接（首连，应收到引导授权 101）---
	a := dialCollab(t, ts, claims)
	grant := a.recv(2 * time.Second)
	if grant == nil || grant[0] != byte(msgBootstrapGrant) {
		t.Fatalf("首连客户端应收到引导授权消息(101)，得到 %v", grant)
	}

	// A 发送 SyncStep1，房间日志为空 → 应收到空 SyncStep2 [0,1,2,0,0]
	a.send(mustHex(t, "00000100"))
	reply := a.recv(2 * time.Second)
	if reply == nil || !bytes.Equal(reply, []byte{0, 1, 2, 0, 0}) {
		t.Fatalf("空房间应回复空 SyncStep2 [0,1,2,0,0]，得到 %v", reply)
	}

	// A 发送内容更新（真实 yjs update）
	updateMsg := mustHex(t, itUpdateHex)
	a.send(updateMsg)

	// --- 客户端 B 连接（非首连，不应收到引导授权）---
	b := dialCollab(t, ts, claims)
	msgs := b.drain(300 * time.Millisecond)
	if len(msgs) != 0 {
		for _, m := range msgs {
			if m[0] == byte(msgBootstrapGrant) {
				t.Fatal("非首连客户端不应收到引导授权")
			}
		}
	}

	// B 发送 SyncStep1 → 应回放 A 的更新
	b.send(mustHex(t, "00000100"))
	msgs = b.drain(time.Second)
	if !b.hasMsg(msgs, "0001") {
		t.Fatalf("B 应收到 SyncStep2 回放，得到 %d 条消息", len(msgs))
	}
	found := false
	want, _ := hex.DecodeString("20" + strings.TrimPrefix(itUpdateHex, "000220")) // 长度前缀 + 载荷
	for _, m := range msgs {
		if len(m) > 2 && bytes.Equal(m[2:], want) {
			found = true
		}
	}
	if !found {
		t.Fatal("回放的 SyncStep2 应包含 A 的更新载荷")
	}

	// A 发送 awareness（真实编码），B 应收到转发
	awareMsg := mustHex(t, itAwarenessHex)
	a.send(awareMsg)
	msgs = b.drain(time.Second)
	if !b.hasMsg(msgs, itAwarenessHex) {
		t.Fatal("B 应收到 A 的 awareness 转发")
	}

	// A 再连一个新客户端 C（迟到者）→ 应收到缓存的 awareness
	c := dialCollab(t, ts, claims)
	msgs = c.drain(time.Second)
	if !c.hasMsg(msgs, itAwarenessHex) {
		t.Fatalf("迟到者 C 应收到缓存的 awareness，收到 %d 条", len(msgs))
	}

	// --- A 断开 → B/C 应收到 A 的 awareness 移除消息（clientID、clock+1）---
	a.ws.Close()
	removeHex := "011001ae9df4820804510 46e756c6c" // [1, len=16? ...] 结构在断言中动态验证
	_ = removeHex
	msgs = b.drain(2 * time.Second)
	var removeMsg []byte
	for _, m := range msgs {
		if len(m) >= 3 && m[0] == byte(msgAwareness) {
			removeMsg = m
		}
	}
	if removeMsg == nil {
		t.Fatal("A 断开后 B 应收到 awareness 移除消息")
	}
	// 验证移除消息结构：varuint(1) + varuint8Array(count=1, clientID, clock+1, "null")
	pos := 0
	if mt, _ := readVarUint(removeMsg, &pos); mt != msgAwareness {
		t.Fatal("移除消息类型错误")
	}
	payload, err := readVarString(removeMsg, &pos)
	if err != nil {
		t.Fatalf("移除消息载荷解析失败: %v", err)
	}
	p := 0
	if cnt, _ := readVarUint(payload, &p); cnt != 1 {
		t.Fatal("移除消息应包含 1 个客户端")
	}
	if cid, _ := readVarUint(payload, &p); cid != itClientID {
		t.Fatalf("移除的 clientID 不匹配: %d", cid)
	}
	if clock, _ := readVarUint(payload, &p); clock != fixtureClock+1 {
		t.Fatalf("移除的 clock 应为 %d: %d", fixtureClock+1, clock)
	}
	if state, _ := readVarString(payload, &p); string(state) != "null" {
		t.Fatalf("移除状态应为 null: %q", string(state))
	}

	// --- 房间清理：B、C 断开后房间应销毁，新连接又成为首连 ---
	b.ws.Close()
	c.ws.Close()
	time.Sleep(300 * time.Millisecond)

	d := dialCollab(t, ts, claims)
	grant = d.recv(2 * time.Second)
	if grant == nil || grant[0] != byte(msgBootstrapGrant) {
		t.Fatal("房间销毁后新连接应再次成为首连（收到 101）")
	}
}
