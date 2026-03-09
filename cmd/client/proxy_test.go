package main

import "testing"

func TestResolveRelayProxyUsesExplicitProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("WS_PROXY", "")
	t.Setenv("WSS_PROXY", "")

	proxyURL, err := resolveRelayProxy("wss://deskgo.zty8.cn/api/desktop/demo", "http://proxy.internal:8080")
	if err != nil {
		t.Fatalf("resolveRelayProxy returned error: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://proxy.internal:8080" {
		t.Fatalf("expected explicit proxy to win, got %v", proxyURL)
	}
}

func TestResolveRelayProxyUsesWSAndWSSProxyEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("WS_PROXY", "http://ws-proxy.internal:8080")
	t.Setenv("WSS_PROXY", "http://wss-proxy.internal:8443")

	wsProxy, err := resolveRelayProxy("ws://relay.internal/api/desktop/demo", "")
	if err != nil {
		t.Fatalf("resolve ws proxy: %v", err)
	}
	if wsProxy == nil || wsProxy.String() != "http://ws-proxy.internal:8080" {
		t.Fatalf("expected WS_PROXY, got %v", wsProxy)
	}

	wssProxy, err := resolveRelayProxy("wss://relay.internal/api/desktop/demo", "")
	if err != nil {
		t.Fatalf("resolve wss proxy: %v", err)
	}
	if wssProxy == nil || wssProxy.String() != "http://wss-proxy.internal:8443" {
		t.Fatalf("expected WSS_PROXY, got %v", wssProxy)
	}
}

func TestResolveRelayProxyUsesHTTPProxyEnvForWebSocketSchemes(t *testing.T) {
	t.Setenv("WS_PROXY", "")
	t.Setenv("WSS_PROXY", "")
	t.Setenv("HTTP_PROXY", "http://http-proxy.internal:8080")
	t.Setenv("HTTPS_PROXY", "http://https-proxy.internal:8443")

	wsProxy, err := resolveRelayProxy("ws://relay.internal/api/desktop/demo", "")
	if err != nil {
		t.Fatalf("resolve ws proxy: %v", err)
	}
	if wsProxy == nil || wsProxy.String() != "http://http-proxy.internal:8080" {
		t.Fatalf("expected HTTP_PROXY for ws, got %v", wsProxy)
	}

	wssProxy, err := resolveRelayProxy("wss://relay.internal/api/desktop/demo", "")
	if err != nil {
		t.Fatalf("resolve wss proxy: %v", err)
	}
	if wssProxy == nil || wssProxy.String() != "http://https-proxy.internal:8443" {
		t.Fatalf("expected HTTPS_PROXY for wss, got %v", wssProxy)
	}
}
