package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"gululoop/internal/config"
)

// 日志文件与轮转参数
const (
	logFileName   = "gulu.log" // 日志文件名（位于 cfg.LogDir 下）
	maxLogSizeMB  = 10         // 单文件最大体积（MB），超过后轮转
	maxLogBackups = 5          // 轮转后保留的历史文件数
	maxLogAgeDays = 7          // 历史文件最大保留天数（0 为不限）
)

var (
	closeMu sync.Mutex
	lj      *lumberjack.Logger // 当前 lumberjack 写句柄，供 Close 释放
)

// New 初始化 slog 日志器并设为默认（slog.SetDefault）：
//
//  1. 文件输出：cfg.LogDir/gulu.log，由 lumberjack 轮转
//     （单文件 10MB、最多保留 5 个历史文件、保留 7 天、gzip 压缩、时间戳用本地时区）
//  2. 日志格式与级别来自 cfg（json | text；debug | info | warn | error）
//  3. 同时写 stderr，便于前台调试
//
// 返回的 *slog.Logger 与 slog.Default() 为同一实例；重复调用 New 会覆盖默认实例，
// 旧句柄通过 Close 释放。进程退出前可调用 Close。
func New(cfg *config.Config) (*slog.Logger, error) {
	if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录 %s: %w", cfg.LogDir, err)
	}

	w := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, logFileName),
		MaxSize:    maxLogSizeMB,
		MaxBackups: maxLogBackups,
		MaxAge:     maxLogAgeDays,
		Compress:   true,
		LocalTime:  true,
	}

	level, err := parseLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}

	var handler slog.Handler
	switch strings.ToLower(cfg.LogFormat) {
	case "text":
		handler = slog.NewTextHandler(io.MultiWriter(w, os.Stderr), &slog.HandlerOptions{Level: level})
	default: // json（config 层已校验）
		handler = slog.NewJSONHandler(io.MultiWriter(w, os.Stderr), &slog.HandlerOptions{Level: level})
	}

	ln := slog.New(handler)
	slog.SetDefault(ln)

	closeMu.Lock()
	lj = w
	closeMu.Unlock()

	return ln, nil
}

// Close 释放日志文件句柄（进程退出前调用，幂等）。
func Close() error {
	closeMu.Lock()
	defer closeMu.Unlock()
	if lj == nil {
		return nil
	}
	err := lj.Close()
	lj = nil
	return err
}

// parseLevel 将配置级别字符串映射为 slog.Level。
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("无效的日志级别 %q: 仅支持 debug | info | warn | error", s)
	}
}
