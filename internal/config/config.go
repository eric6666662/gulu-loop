package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAddr       = "127.0.0.1:1112" // 默认只监听本机
	defaultLLMTimeout = 60 * time.Second // LLM 调用硬超时（01-需求 约束）
	defaultLogFormat  = "json"
	defaultLogLevel   = "info"
)

// Config 全局配置
type Config struct {
	Addr      string // 监听地址
	DataDir   string // 数据目录
	LogDir    string // 日志目录
	LogFormat string // 日志格式 json | text
	LogLevel  string // 日志级别 debug | info | warn | error

	LLMTimeout time.Duration // LLM 调用硬超时（流式与拉取模型共用）
}

// def 默认配置
func def() *Config {
	homeDir, err := os.UserHomeDir()

	if err != nil || homeDir == "" {
		homeDir = "."
	}

	dataDir := filepath.Join(homeDir, ".gulu")

	return &Config{
		Addr:       defaultAddr,
		DataDir:    dataDir,
		LogDir:     filepath.Join(dataDir, "logs"),
		LogFormat:  defaultLogFormat,
		LogLevel:   defaultLogLevel,
		LLMTimeout: defaultLLMTimeout,
	}
}

func Parse() (*Config, error) {
	return parse(os.Args[1:])
}

// ErrHelp 命令行传入 -h / --help 时返回，main 可据此以 0 退出
var ErrHelp = errors.New("help requested")

// parse 从启动命令（os.Args[1:]）读取配置，未指定的项使用默认值。
//
//	-addr        监听地址（默认 127.0.0.1:1112）
//	-data-dir    数据目录（默认 ~/.gulu）
//	-log-dir     日志目录（默认 <data-dir>/logs）
//	-log-format  日志格式 json | text（默认 json）
//	-log-level   日志级别 debug | info | warn | error（默认 info）
//	-llm-timeout LLM 调用硬超时（默认 60s；流式与拉取模型共用）
func parse(args []string) (*Config, error) {
	base := def()

	fs := flag.NewFlagSet("gulu-loop", flag.ContinueOnError)

	addr := fs.String("addr", base.Addr, "监听地址")
	dataDir := fs.String("data-dir", base.DataDir, "数据目录")
	logDir := fs.String("log-dir", "", "日志目录（默认 <data-dir>/logs）")
	logFormat := fs.String("log-format", base.LogFormat, "日志格式 json | text")
	logLevel := fs.String("log-level", base.LogLevel, "日志级别 debug | info | warn | error")
	llmTimeout := fs.Duration("llm-timeout", base.LLMTimeout, "LLM 调用硬超时（默认 60s）")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, ErrHelp
		}
		return nil, fmt.Errorf("解析命令行参数: %w", err)
	}

	cfg := &Config{
		Addr:       *addr,
		DataDir:    *dataDir,
		LogDir:     *logDir,
		LogFormat:  *logFormat,
		LogLevel:   *logLevel,
		LLMTimeout: *llmTimeout,
	}
	if cfg.LogDir == "" {
		cfg.LogDir = filepath.Join(cfg.DataDir, "logs")
	}

	return cfg, cfg.validate()
}

// validate 校验取值合法性并规范化
func (c *Config) validate() error {
	if c.Addr == "" {
		return errors.New("-addr 不能为空")
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("无效的 log-format %q: 仅支持 json | text", c.LogFormat)
	}
	c.LogLevel = strings.ToLower(c.LogLevel)
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("无效的 log-level %q: 仅支持 debug | info | warn | error", c.LogLevel)
	}
	return nil
}
