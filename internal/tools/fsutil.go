package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// NormalizeMingwPath 将 MINGW64/Git Bash 风格路径 /d/project/... 转换为 Windows 路径 D:\project\...
//
// LLM 在 Windows 上调 bash(pwd) 时拿到的是 /d/project/... 格式，
// 后续生成路径也可能沿用此格式。filepath.Abs 无法正确处理它（会解析为当前盘的 \d\...）。
// 非 Windows 平台或不匹配的格式原样返回。
func NormalizeMingwPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	if len(p) >= 3 && p[0] == '/' && isASCIILetter(p[1]) && p[2] == '/' {
		return strings.ToUpper(string(p[1])) + ":" + filepath.FromSlash(p[2:])
	}
	return p
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isInAllowedDirs 判断目标路径是否在白名单目录中
//
// 白名单支持目录前缀匹配（Windows 上大小写不敏感）。
// 会解析符号链接以防止符号链接路径穿越攻击。
func isInAllowedDirs(target string, allowed []string) (bool, error) {
	if target == "" {
		return false, errors.New("目标路径为空")
	}

	abs, err := filepath.Abs(NormalizeMingwPath(target))
	if err != nil {
		return false, err
	}
	abs = filepath.Clean(abs)

	// 解析符号链接，防止符号链接指向白名单外路径
	resolved, err := resolveSymlinks(abs)
	if err != nil {
		return false, err
	}

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

		// 解析白名单目录的符号链接（目录本身可能是符号链接）
		rootResolved, err := filepath.EvalSymlinks(root)
		if err == nil {
			root = rootResolved
		}
		// 解析失败（目录不存在）则使用 Clean 后的路径

		// 统一大小写（Windows/macOS 文件系统不区分大小写）
		cmpAbs, cmpRoot := resolved, root
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

// resolveSymlinks 解析路径中的符号链接。如果路径不存在（如要写入的新文件），
// 则解析已存在的最近父目录，再拼接剩余部分。
func resolveSymlinks(abs string) (string, error) {
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	// 路径不存在：向上找到存在的最近父目录并解析
	parent := filepath.Dir(abs)
	parentResolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		// 连父目录都无法解析，回退到 Clean 后的路径
		return abs, nil
	}
	return filepath.Join(parentResolved, filepath.Base(abs)), nil
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
