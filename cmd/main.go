// coding-agent CLI 入口
//
// 启动顺序：
//  1. 解析 --env / -e flag（在 cobra 接管前用标准库 flag 抢出来）
//  2. 加载 .env，把变量塞进 os.Environ()
//  3. 调用 cli.Execute() 跑 cobra
//
// .env 加载策略：
//   - 找不到文件不报错（允许用户只用 shell env）
//   - 已存在的环境变量不会被 .env 覆盖（godotenv.Load 语义）
//   - --env 留空时默认加载当前目录的 ".env"
//   - --env -  可禁用 .env 加载
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/wsx864321/coding-agent/cmd/cli"
)

func main() {
	// 先用标准库 flag 把 --env/-e 抢出来
	envPath, rest, err := parseEnvFlag(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}
	os.Args = append([]string{os.Args[0]}, rest...)

	if err := loadDotenv(envPath); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	cli.Execute()
}

// parseEnvFlag 用标准库 flag 抢出 --env/-e，剩余 args 透传
func parseEnvFlag(args []string) (envPath string, rest []string, err error) {
	fs := flag.NewFlagSet("coding-agent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&envPath, "env", ".env", "dotenv 文件路径，\"-\" 表示禁用")
	fs.StringVar(&envPath, "e", ".env", "dotenv 文件路径（简写）")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	return envPath, fs.Args(), nil
}

// loadDotenv 加载 envPath 指定的 .env 文件
func loadDotenv(envPath string) error {
	if envPath == "-" {
		return nil
	}
	if envPath == "" {
		envPath = ".env"
	}

	var err error
	if envPath == ".env" {
		err = godotenv.Load()
	} else {
		err = godotenv.Load(envPath)
	}
	if err == nil {
		return nil
	}
	// 找不到文件：静默
	if os.IsNotExist(err) || isEnvNotFoundErr(err) {
		return nil
	}
	return fmt.Errorf("加载 %s 失败: %w", envPath, err)
}

func isEnvNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, sub := range []string{"no such file", "cannot find", "not found"} {
		if contains(msg, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
