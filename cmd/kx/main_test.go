package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	tests := []struct {
		name    string
		version string
		info    *debug.BuildInfo
		want    string
	}{
		{"ldflags wins", "2.1.0", info("v9.9.9"), "2.1.0"},
		{"go install", "dev", info("v2.1.0"), "v2.1.0"},
		{"local build", "dev", info("(devel)"), "dev"},
		{"empty version", "dev", info(""), "dev"},
		{"no build info", "dev", nil, "dev"},
	}
	for _, tt := range tests {
		if got := resolveVersion(tt.version, tt.info); got != tt.want {
			t.Errorf("%s: resolveVersion = %q, want %q", tt.name, got, tt.want)
		}
	}
}
