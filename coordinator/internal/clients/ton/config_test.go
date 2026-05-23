package tonclient

import "testing"

func TestLocalConfigPath(t *testing.T) {
	tests := []struct {
		in      string
		wantOk  bool
		wantPath string
	}{
		{in: "file:///etc/ton/config.json", wantOk: true, wantPath: "/etc/ton/config.json"},
		{in: "/etc/ton/config.json", wantOk: true, wantPath: "/etc/ton/config.json"},
		{in: "./deploy/ton.config.json", wantOk: true, wantPath: "./deploy/ton.config.json"},
		{in: "https://ton.org/global.config.json", wantOk: false},
		{in: "http://127.0.0.1/config.json", wantOk: false},
	}
	for _, tt := range tests {
		path, ok := localConfigPath(tt.in)
		if ok != tt.wantOk {
			t.Fatalf("%q: ok=%v want %v", tt.in, ok, tt.wantOk)
		}
		if ok && path != tt.wantPath {
			t.Fatalf("%q: path=%q want %q", tt.in, path, tt.wantPath)
		}
	}
}
