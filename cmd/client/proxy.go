package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func resolveRelayProxy(serverURL string, explicitProxy string) (*url.URL, error) {
	if proxyValue := strings.TrimSpace(explicitProxy); proxyValue != "" {
		proxyURL, err := url.Parse(proxyValue)
		if err != nil {
			return nil, fmt.Errorf("解析显式代理地址失败: %w", err)
		}
		if err := validateProxyURL(proxyURL); err != nil {
			return nil, fmt.Errorf("显式代理地址无效: %w", err)
		}
		return proxyURL, nil
	}

	relayURL, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("解析 Relay 地址失败: %w", err)
	}

	if schemeProxy := strings.TrimSpace(relaySchemeProxyEnv(relayURL.Scheme)); schemeProxy != "" {
		proxyURL, err := url.Parse(schemeProxy)
		if err != nil {
			return nil, fmt.Errorf("解析环境变量代理地址失败: %w", err)
		}
		if err := validateProxyURL(proxyURL); err != nil {
			return nil, fmt.Errorf("环境变量代理地址无效: %w", err)
		}
		return proxyURL, nil
	}

	proxyLookupURL := *relayURL
	switch proxyLookupURL.Scheme {
	case "ws":
		proxyLookupURL.Scheme = "http"
	case "wss":
		proxyLookupURL.Scheme = "https"
	}

	proxyURL, err := http.ProxyFromEnvironment(&http.Request{URL: &proxyLookupURL})
	if err != nil {
		return nil, fmt.Errorf("从环境变量读取代理失败: %w", err)
	}
	return proxyURL, nil
}

func validateProxyURL(proxyURL *url.URL) error {
	if proxyURL == nil {
		return nil
	}
	if proxyURL.Scheme == "" {
		return fmt.Errorf("缺少代理协议")
	}
	if proxyURL.Host == "" {
		return fmt.Errorf("缺少代理主机")
	}
	return nil
}

func relaySchemeProxyEnv(scheme string) string {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "ws":
		return firstNonEmptyEnv("WS_PROXY", "ws_proxy")
	case "wss":
		return firstNonEmptyEnv("WSS_PROXY", "wss_proxy")
	default:
		return ""
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
