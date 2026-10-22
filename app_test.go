package main

import "testing"

func TestVersionNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.1", "0.1.0", true},   // 补丁升级
		{"v0.2.0", "0.1.9", true},   // 次版本升级
		{"v1.0.0", "0.9.9", true},   // 大版本升级
		{"0.1.0", "0.1.0", false},   // 相同
		{"v0.1.0", "0.1.0", false},  // 相同（v 前缀差异）
		{"0.1.0", "v0.1.1", false},  // 本地更新
		{"v0.1.0-beta", "0.1.0", false}, // 预发布不视为新版本
		{"v0.1.0", "0.1.0-beta", false}, // 本地预发布也不视为旧版本
	}
	for _, c := range cases {
		if got := versionNewer(c.latest, c.current); got != c.want {
			t.Errorf("versionNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}
