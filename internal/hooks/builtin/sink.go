package builtin

import (
	"fmt"
	"io"
	"os"
)

// Sink 是各 hook 的可选输出目标；nil 时默认走 os.Stderr
//
// 之所以是独立类型而不是直接用 io.Writer，是为了未来支持彩色 / 标签前缀
// 等更复杂的输出策略时不影响调用方代码
type Sink struct {
	W io.Writer
}

// Fprintf 写入一行
func (s *Sink) Fprintf(format string, a ...any) {
	w := s.W
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, format, a...)
}
