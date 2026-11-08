package collab

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// 测试向量由 collab_client/scripts/gen-fixtures.mjs 生成（真实 yjs/y-protocols/lib0 编码）
const (
	fixtureAwarenessHex = "015901ae9df4820802517b2275736572223a7b226e616d65223a22e5bca0e4b889222c22636f6c6f72223a2223333062636564227d2c22637572736f72223a7b22616e63686f72223a6e756c6c2c2268656164223a6e756c6c7d7d"
	fixtureClientID     = uint64(2153582254)
	fixtureClock        = uint64(2)
	fixtureSyncStep1Hex = "00000100"
	fixtureUpdateHex    = "0002200101ae9df4820800040107636f6e74656e740c68656c6c6f206d696e646f6300"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	return b
}

func TestParseAwareness(t *testing.T) {
	msg := mustHex(t, fixtureAwarenessHex)
	clients := parseAwareness(msg)
	if len(clients) != 1 {
		t.Fatalf("期望解析出 1 个客户端，实际 %d", len(clients))
	}
	if clients[0].client != fixtureClientID {
		t.Fatalf("clientID 不匹配: got %d want %d", clients[0].client, fixtureClientID)
	}
	if clients[0].clock != fixtureClock {
		t.Fatalf("clock 不匹配: got %d want %d", clients[0].clock, fixtureClock)
	}
}

func TestParseSyncHeader(t *testing.T) {
	// sync step1：varuint(0) + varuint(0) + varuint8Array(SV)
	step1 := mustHex(t, fixtureSyncStep1Hex)
	sub, pos, err := parseSyncHeader(step1)
	if err != nil || sub != syncStep1 {
		t.Fatalf("step1 解析失败: sub=%d err=%v", sub, err)
	}
	sv, err := readVarString(step1, &pos)
	if err != nil || len(sv) != 1 || sv[0] != 0 {
		t.Fatalf("SV 载荷解析失败: %v %v", sv, err)
	}

	// sync update：varuint(0) + varuint(2) + varuint8Array(update)
	upd := mustHex(t, fixtureUpdateHex)
	sub, pos, err = parseSyncHeader(upd)
	if err != nil || sub != syncUpdate {
		t.Fatalf("update 解析失败: sub=%d err=%v", sub, err)
	}
	payload, err := readVarString(upd, &pos)
	if err != nil {
		t.Fatalf("update 载荷解析失败: %v", err)
	}
	if len(payload) != 0x20 {
		t.Fatalf("update 载荷长度不匹配: got %d want 32", len(payload))
	}

	// 用带长度前缀的载荷重构 SyncStep2，应得到 [0,1]+载荷
	built := buildSyncStep2(upd[pos-1-len(payload):])
	// upd[pos-1-len(payload):] 即从长度前缀开始的完整载荷
	want := append([]byte{byte(msgSync), byte(syncStep2)}, upd[pos-1-len(payload):]...)
	if !bytes.Equal(built, want) {
		t.Fatalf("buildSyncStep2 输出不匹配")
	}
	_ = payload
}

func TestBuildAwarenessRemove(t *testing.T) {
	clients := []awareClient{{client: fixtureClientID, clock: fixtureClock}}
	msg := buildAwarenessRemove(clients)

	// 结构校验：varuint(1) + varuint8Array(count=1, clientID, clock+1, "null")
	pos := 0
	if mt, err := readVarUint(msg, &pos); err != nil || mt != msgAwareness {
		t.Fatalf("消息类型错误: %d %v", mt, err)
	}
	payload, err := readVarString(msg, &pos)
	if err != nil {
		t.Fatalf("载荷长度前缀解析失败: %v", err)
	}
	p := 0
	if cnt, err := readVarUint(payload, &p); err != nil || cnt != 1 {
		t.Fatalf("客户端数量错误: %d %v", cnt, err)
	}
	if cid, err := readVarUint(payload, &p); err != nil || cid != fixtureClientID {
		t.Fatalf("clientID 错误: %d %v", cid, err)
	}
	if clock, err := readVarUint(payload, &p); err != nil || clock != fixtureClock+1 {
		t.Fatalf("clock 错误: %d %v", clock, err)
	}
	if state, err := readVarString(payload, &p); err != nil || string(state) != "null" {
		t.Fatalf("state 错误: %q %v", string(state), err)
	}

	// 自身的解析器也应能解析该消息
	parsed := parseAwareness(msg)
	if len(parsed) != 1 || parsed[0].client != fixtureClientID {
		t.Fatalf("parseAwareness(buildAwarenessRemove) 不一致: %+v", parsed)
	}
}

func TestParseSyncHeaderMalformed(t *testing.T) {
	cases := [][]byte{nil, {0}, {byte(msgSync)}, {byte(msgAwareness), byte(syncStep1)}}
	for i, c := range cases {
		if _, _, err := parseSyncHeader(c); err == nil {
			t.Fatalf("case %d 应当解析失败", i)
		}
	}
	if s := string(rune(0)); len(s) != 1 {
		t.Fatal("unreachable")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	key := []byte("test-app-key")
	claims := Claims{MemberID: 1, BookID: 2, DocID: 3, Identify: "abc"}

	token, err := SignToken(claims, key)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if strings.Count(token, ".") != 1 {
		t.Fatalf("token 格式错误: %s", token)
	}

	got, err := ParseToken(token, key)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if *got != (Claims{MemberID: 1, BookID: 2, DocID: 3, Identify: "abc", Expires: got.Expires}) {
		t.Fatalf("claims 不匹配: %+v", *got)
	}
	if got.Expires == 0 {
		t.Fatal("未自动填充过期时间")
	}

	// 错误密钥
	if _, err := ParseToken(token, []byte("wrong")); err != ErrInvalidToken {
		t.Fatalf("错误密钥应返回 ErrInvalidToken: %v", err)
	}
	// 过期
	expired := Claims{MemberID: 1, Expires: 1}
	tok2, _ := SignToken(expired, key)
	if _, err := ParseToken(tok2, key); err != ErrExpiredToken {
		t.Fatalf("过期 token 应返回 ErrExpiredToken: %v", err)
	}
	// 篡改
	if _, err := ParseToken(tok2+"x", key); err == nil {
		t.Fatal("畸形 token 应当报错")
	}
}
