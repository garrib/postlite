// Package config 解析运行时配置（环境变量），约定见 docs/design.md §4.2。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Version 与发版号保持一致。
const Version = "0.3.0"

type Config struct {
	Port        int    // 实际监听起点端口（占用自动 +1 由 main 处理）
	DataDir     string // 历史存储目录
	OpenBrowser bool
}

func Load() (Config, error) {
	port := 7300
	if v := os.Getenv("POSTLITE_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return Config{}, fmt.Errorf("POSTLITE_PORT 非法: %q", v)
		}
		port = n
	}

	dataDir := os.Getenv("POSTLITE_DATA_DIR")
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Config{}, fmt.Errorf("无法确定用户配置目录: %w", err)
		}
		dataDir = filepath.Join(base, "postlite")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return Config{}, fmt.Errorf("创建数据目录失败: %w", err)
	}

	return Config{
		Port:        port,
		DataDir:     dataDir,
		OpenBrowser: os.Getenv("POSTLITE_NO_OPEN") != "1",
	}, nil
}
