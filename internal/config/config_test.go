package config

import (
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	cfg, err := parse([]string{"--addr=0.0.0.0:9000", "--data-dir=/tmp/xgulu-test"})

	if err != nil {
		t.Fatalf("Parse 返回错误: %v", err)
	}

	if cfg.Addr != "0.0.0.0:9000" {
		t.Errorf("Addr = %q，期望被旗标覆盖", cfg.Addr)
	}
	if cfg.DataDir != "/tmp/xgulu-test" {
		t.Errorf("DataDir = %q，期望 /tmp/xgulu-test", cfg.DataDir)
	}

	if cfg.LogDir != filepath.Join("/tmp/xgulu-test", "logs") {
		t.Errorf("LogDir = %q，期望随 DataDir 迁移", cfg.LogDir)
	}
}
