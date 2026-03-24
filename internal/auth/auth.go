package auth

import (
	"errors"
)

var errDangerousEscape = errors.New("dangerous escape sequence")

// SanitizeTerminalInput 保持尽量宽松，只拒绝明显不该进入 PTY 的内容。
func SanitizeTerminalInput(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) > 65536 {
		return nil, errors.New("input too large")
	}

	for i := 0; i < len(data); i++ {
		if data[i] == 0 {
			return nil, errors.New("null byte is not allowed")
		}
		if data[i] == 0x1b && i+1 < len(data) {
			switch data[i+1] {
			case ']', 'P', 'X', '^', '_':
				return nil, errDangerousEscape
			}
		}
	}

	return append([]byte(nil), data...), nil
}
