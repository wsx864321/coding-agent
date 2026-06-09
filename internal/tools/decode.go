package tools

import (
	"encoding/json"
	"errors"
	"fmt"
)

// decodeArgs 将 map[string]any 解析为目标结构体
func decodeArgs(args map[string]any, target any) error {
	if args == nil {
		return errors.New("参数为空")
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("序列化参数失败: %w", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("解析参数失败: %w", err)
	}
	return nil
}
