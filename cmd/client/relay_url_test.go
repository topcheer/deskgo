package main

import "testing"

func TestNormalizeSessionID(t *testing.T) {
	if got := normalizeSessionID(" WiN11Studio "); got != "win11studio" {
		t.Fatalf("expected lower-cased session id, got %q", got)
	}
}

func TestNormalizeRelayServerURLSupportsHTTPAndHTTPS(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "http base URL",
			raw:  "http://192.168.31.105:8082",
			want: "ws://192.168.31.105:8082/api/desktop",
		},
		{
			name: "https base URL",
			raw:  "https://deskgo.example.com",
			want: "wss://deskgo.example.com/api/desktop",
		},
		{
			name: "existing websocket path",
			raw:  "ws://relay.internal/api/desktop",
			want: "ws://relay.internal/api/desktop",
		},
		{
			name: "nested HTTP path",
			raw:  "http://relay.internal/custom",
			want: "ws://relay.internal/custom/api/desktop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRelayServerURL(tt.raw)
			if err != nil {
				t.Fatalf("normalizeRelayServerURL returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestRelayWebBaseURL(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"ws://192.168.31.105:8082/api/desktop", "http://192.168.31.105:8082"},
		{"wss://deskgo.example.com/api/desktop", "https://deskgo.example.com"},
		{"http://relay.internal/custom/api/desktop", "http://relay.internal/custom"},
	}

	for _, tt := range tests {
		if got := relayWebBaseURL(tt.raw); got != tt.want {
			t.Fatalf("relayWebBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestNormalizeRelayServerURLRejectsUnsupportedScheme(t *testing.T) {
	if _, err := normalizeRelayServerURL("ftp://relay.internal"); err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
}
