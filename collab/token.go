package collab

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// 协同连接凭证错误
var (
	ErrInvalidToken   = errors.New("collab: invalid token")
	ErrExpiredToken   = errors.New("collab: token expired")
	ErrTokenMismatch  = errors.New("collab: token does not match requested document")
	ErrTokenMalformed = errors.New("collab: malformed token")
)

// Claims 协同连接凭证内容。
// 由 CollabController.Token 基于当前登录会话签发，WebSocket 升级时校验，
// 避免在原生 Handler 中重复实现 beego 会话鉴权。
type Claims struct {
	MemberID int    `json:"m"` // 签发时的用户 ID
	BookID   int    `json:"b"` // 项目 ID
	DocID    int    `json:"d"` // 文档 ID
	Identify string `json:"k"` // 项目标识
	Expires  int64  `json:"e"` // 过期时间（Unix 秒）
}

// SignToken 使用 HMAC-SHA256 签发 token，格式为 base64url(payload).base64url(mac)。
func SignToken(claims Claims, key []byte) (string, error) {
	if claims.Expires == 0 {
		claims.Expires = time.Now().Add(DefaultTokenTTL).Unix()
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	sum := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sum), nil
}

// ParseToken 校验并解析 token，同时检查有效期。
func ParseToken(token string, key []byte) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrTokenMalformed
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrTokenMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenMalformed
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrTokenMalformed
	}
	if time.Now().Unix() > claims.Expires {
		return nil, ErrExpiredToken
	}
	return &claims, nil
}
