package session

import (
	"regexp"
)

// sessionIDPattern 用于从 metadata.user_id 中提取 session ID 的正则表达式
var sessionIDPattern = regexp.MustCompile(`session_([a-f0-9-]{36})`)

// ExtractSessionIDFromMetadata 从 metadata.user_id 字符串中提取 session ID
// 仅处理旧字符串格式: user_{64位字符串}_account__session_{uuid}
func ExtractSessionIDFromMetadata(userID string) string {
	if userID == "" {
		return ""
	}
	matches := sessionIDPattern.FindStringSubmatch(userID)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
