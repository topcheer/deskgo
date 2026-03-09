package main

import (
	"fmt"
	"net/url"
	"strings"
)

func normalizeSessionID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeRelayServerURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("relay URL 不能为空")
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("解析 URL 失败: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("不支持的 Relay URL scheme: %s", u.Scheme)
	}

	path := strings.TrimRight(u.Path, "/")
	switch {
	case path == "":
		u.Path = "/api/desktop"
	case strings.HasSuffix(path, "/api/desktop"):
		u.Path = path
	default:
		u.Path = path + "/api/desktop"
	}

	return u.String(), nil
}

func relayWebBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return raw
	}

	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
	default:
		return raw
	}

	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}
