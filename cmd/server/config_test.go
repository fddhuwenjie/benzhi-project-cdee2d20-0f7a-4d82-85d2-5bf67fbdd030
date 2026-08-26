package main

import "testing"

func TestParseConfigUsesSafeDefaultsAndPort(t *testing.T) {
	t.Setenv("PORT", "")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.address != defaultAddress {
		t.Fatalf("默认地址错误: %s", cfg.address)
	}
	t.Setenv("PORT", "19123")
	cfg, err = parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.address != "127.0.0.1:19123" {
		t.Fatalf("PORT 地址错误: %s", cfg.address)
	}
}

func TestParseConfigRejectsNonLoopback(t *testing.T) {
	t.Setenv("PORT", "")
	if _, err := parseConfig([]string{"-addr=0.0.0.0:19081"}); err == nil {
		t.Fatal("非回环地址未被拒绝")
	}
}

func TestFullSelfCheck(t *testing.T) {
	if err := runSelfCheck(t.Context(), config{address: "127.0.0.1:0", selfCheck: true}); err != nil {
		t.Fatal(err)
	}
}
