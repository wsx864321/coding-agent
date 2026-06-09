package tools

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// isInAllowedDirs 判断目标路径是否在白名单目录中
//
// 白名单支持目录前缀匹配（Windows 上大小写不敏感）。
func isInAllowedDirs(target string, allowed []string) (bool, error) {
	if target == "" {
		return false, errors.New("目标路径为空")
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	abs = filepath.Clean(abs)

	caseInsensitive := isCaseInsensitiveFS()

	for _, dir := range allowed {
		if dir == "" {
			continue
		}
		root, err := filepath.Abs(dir)
		if err != nil {
			return false, err
		}
		root = filepath.Clean(root)

		// 统一大小写（Windows/macOS 文件系统不区分大小写）
		cmpAbs, cmpRoot := abs, root
		if caseInsensitive {
			cmpAbs = strings.ToLower(cmpAbs)
			cmpRoot = strings.ToLower(cmpRoot)
		}

		if cmpAbs == cmpRoot {
			return true, nil
		}
		// 必须是 root 的子目录
		rel, err := filepath.Rel(cmpRoot, cmpAbs)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true, nil
		}
	}
	return false, nil
}

// isCaseInsensitiveFS 判断当前操作系统文件系统是否大小写不敏感
func isCaseInsensitiveFS() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

// fileExists 判断路径是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
