package main

import (
	"os"
	"sort"
	"strings"
)

type downloadArtifact struct {
	FileName string
	URL      string
	Arch     string
}

type downloadGroup struct {
	Kind          string
	OS            string
	TitleZH       string
	TitleEN       string
	DescriptionZH string
	DescriptionEN string
	Artifacts     []downloadArtifact
}

type siteDownloads struct {
	DesktopDownloads     []downloadGroup
	RelayDownloads       []downloadGroup
	DesktopArtifactCount int
	RelayArtifactCount   int
	HasChecksums         bool
	ChecksumURL          string
}

type desktopPageStrings struct {
	PageTitle                  string
	BrandEyebrow               string
	BrandTitle                 string
	ControlToggleLabel         string
	StatusConnecting           string
	StatusWaitingClient        string
	StatusStreaming            string
	StatusError                string
	StatusSessionEnded         string
	StatusDisconnected         string
	LoadingConnectingTitle     string
	LoadingWaitingTitle        string
	LoadingErrorTitle          string
	LoadingErrorDetail         string
	LoadingSessionEndedTitle   string
	LoadingSessionEndedDetail  string
	LoadingDisconnectedTitle   string
	LoadingDisconnectedDetail  string
	SessionDetailPrefix        string
	RetryButtonLabel           string
	RetrySessionMessage        string
	DefaultDisconnectReason    string
	DesktopDisconnectedMessage string
	ResolutionLabel            string
	FPSLabel                   string
	LatencyLabel               string
	SessionLabel               string
	QualityLabel               string
	QualityExcellent           string
	QualityGood                string
	QualityFair                string
	QualityPoor                string
}

type parsedDownload struct {
	Kind string
	OS   string
	Arch string
}

func collectSiteDownloads(downloadsDir string) siteDownloads {
	data := siteDownloads{
		ChecksumURL: "/downloads/SHA256SUMS.txt",
	}

	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		return data
	}

	groups := map[string]*downloadGroup{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name == "SHA256SUMS.txt" {
			data.HasChecksums = true
			continue
		}

		parsed, ok := parseDownloadName(name)
		if !ok {
			continue
		}

		key := parsed.Kind + ":" + parsed.OS
		group, exists := groups[key]
		if !exists {
			group = newDownloadGroup(parsed.Kind, parsed.OS)
			groups[key] = group
		}

		group.Artifacts = append(group.Artifacts, downloadArtifact{
			FileName: name,
			URL:      "/downloads/" + name,
			Arch:     archLabel(parsed.Arch),
		})

		if parsed.Kind == "desktop" {
			data.DesktopArtifactCount++
		} else {
			data.RelayArtifactCount++
		}
	}

	for _, group := range groups {
		sort.Slice(group.Artifacts, func(i, j int) bool {
			return archSortOrder(group.Artifacts[i].Arch) < archSortOrder(group.Artifacts[j].Arch)
		})

		if group.Kind == "desktop" {
			data.DesktopDownloads = append(data.DesktopDownloads, *group)
		} else {
			data.RelayDownloads = append(data.RelayDownloads, *group)
		}
	}

	sort.Slice(data.DesktopDownloads, func(i, j int) bool {
		return downloadGroupOrder(data.DesktopDownloads[i].OS) < downloadGroupOrder(data.DesktopDownloads[j].OS)
	})
	sort.Slice(data.RelayDownloads, func(i, j int) bool {
		return downloadGroupOrder(data.RelayDownloads[i].OS) < downloadGroupOrder(data.RelayDownloads[j].OS)
	})

	return data
}

func parseDownloadName(name string) (parsedDownload, bool) {
	trimmed := strings.TrimSuffix(name, ".exe")
	parts := strings.Split(trimmed, "-")
	if len(parts) < 4 || parts[0] != "deskgo" {
		return parsedDownload{}, false
	}

	kind := parts[1]
	if kind != "desktop" && kind != "relay" {
		return parsedDownload{}, false
	}

	osName := parts[2]
	if osName != "darwin" && osName != "linux" && osName != "windows" {
		return parsedDownload{}, false
	}

	arch := strings.Join(parts[3:], "-")
	if arch == "" {
		return parsedDownload{}, false
	}

	return parsedDownload{
		Kind: kind,
		OS:   osName,
		Arch: arch,
	}, true
}

func newDownloadGroup(kind, osName string) *downloadGroup {
	group := &downloadGroup{
		Kind: kind,
		OS:   osName,
	}

	switch kind + ":" + osName {
	case "desktop:darwin":
		group.TitleZH = "macOS Desktop CLI"
		group.TitleEN = "macOS Desktop CLI"
		group.DescriptionZH = "原生桌面捕获与 H.264 优先编码路径，适用于 Intel 与 Apple Silicon Mac。"
		group.DescriptionEN = "Native desktop capture with the H.264-first path for both Intel and Apple Silicon Macs."
	case "desktop:windows":
		group.TitleZH = "Windows Desktop CLI"
		group.TitleEN = "Windows Desktop CLI"
		group.DescriptionZH = "保持与 macOS 接近的一致会话体验，适用于 AMD64 与 ARM64 Windows 设备。"
		group.DescriptionEN = "Session output and input behavior aligned with macOS for both AMD64 and ARM64 Windows devices."
	case "desktop:linux":
		group.TitleZH = "Linux Desktop CLI"
		group.TitleEN = "Linux Desktop CLI"
		group.DescriptionZH = "默认优先使用 H.264（检测到 ffmpeg 时），并支持常见服务器、开发机与 ARM / RISC-V 主机。"
		group.DescriptionEN = "H.264-first by default when ffmpeg is available, with builds for mainstream server, ARM, and RISC-V Linux hosts."
	case "relay:darwin":
		group.TitleZH = "macOS Relay Server"
		group.TitleEN = "macOS Relay Server"
		group.DescriptionZH = "适合本地开发、演示环境以及 Intel / Apple Silicon Mac 上的轻量部署。"
		group.DescriptionEN = "Ideal for local development, demos, and lightweight relay deployments on Intel or Apple Silicon Macs."
	case "relay:windows":
		group.TitleZH = "Windows Relay Server"
		group.TitleEN = "Windows Relay Server"
		group.DescriptionZH = "适用于 Windows 测试环境、内部演示节点或需要本地运行 Relay 的桌面设备。"
		group.DescriptionEN = "Useful for Windows test environments, demo nodes, and local relay deployments on desktop machines."
	default:
		group.TitleZH = "Linux Relay Server"
		group.TitleEN = "Linux Relay Server"
		group.DescriptionZH = "纯 Go 多架构中继二进制，适用于容器宿主机、裸机节点与异构 Linux 服务器。"
		group.DescriptionEN = "Pure-Go multi-architecture relay binaries for containers, bare metal, and heterogeneous Linux server fleets."
	}

	return group
}

func archLabel(arch string) string {
	switch arch {
	case "amd64":
		return "AMD64"
	case "arm64":
		return "ARM64"
	case "armv7":
		return "ARMv7"
	case "riscv64":
		return "RISC-V 64"
	case "ppc64le":
		return "PPC64LE"
	case "s390x":
		return "S390X"
	case "386":
		return "386"
	default:
		return strings.ToUpper(arch)
	}
}

func archSortOrder(arch string) int {
	switch arch {
	case "AMD64":
		return 0
	case "ARM64":
		return 1
	case "ARMv7":
		return 2
	case "RISC-V 64":
		return 3
	case "PPC64LE":
		return 4
	case "S390X":
		return 5
	case "386":
		return 6
	default:
		return 99
	}
}

func downloadGroupOrder(osName string) int {
	switch osName {
	case "darwin":
		return 0
	case "windows":
		return 1
	case "linux":
		return 2
	default:
		return 99
	}
}

func desktopStringsFor(lang string) desktopPageStrings {
	if lang == "en" {
		return desktopPageStrings{
			PageTitle:                  "DeskGo - Remote Desktop Session",
			BrandEyebrow:               "DeskGo Session",
			BrandTitle:                 "Remote desktop streaming",
			ControlToggleLabel:         "Enable control",
			StatusConnecting:           "Connecting...",
			StatusWaitingClient:        "Waiting for client...",
			StatusStreaming:            "Streaming",
			StatusError:                "Connection error",
			StatusSessionEnded:         "Session ended",
			StatusDisconnected:         "Disconnected",
			LoadingConnectingTitle:     "Connecting to the remote desktop...",
			LoadingWaitingTitle:        "Waiting for the desktop client...",
			LoadingErrorTitle:          "Connection problem",
			LoadingErrorDetail:         "The relay connection failed. Please check the network and try again.",
			LoadingSessionEndedTitle:   "Desktop client disconnected",
			LoadingSessionEndedDetail:  "This session has ended. Restart the CLI and reconnect.",
			LoadingDisconnectedTitle:   "Connection lost",
			LoadingDisconnectedDetail:  "Trying to reconnect to the remote desktop...",
			SessionDetailPrefix:        "Session ID: ",
			RetryButtonLabel:           "Reconnect",
			RetrySessionMessage:        "Reconnecting to the session...",
			DefaultDisconnectReason:    "Connection closed",
			DesktopDisconnectedMessage: "The desktop client disconnected and the session ended. Restart the CLI before reconnecting.",
			ResolutionLabel:            "Resolution",
			FPSLabel:                   "Frame rate",
			LatencyLabel:               "Latency",
			SessionLabel:               "Session",
			QualityLabel:               "Quality",
			QualityExcellent:           "Excellent",
			QualityGood:                "Good",
			QualityFair:                "Fair",
			QualityPoor:                "Poor",
		}
	}

	return desktopPageStrings{
		PageTitle:                  "DeskGo - 远程桌面会话",
		BrandEyebrow:               "DeskGo Session",
		BrandTitle:                 "远程桌面串流",
		ControlToggleLabel:         "启用控制",
		StatusConnecting:           "连接中...",
		StatusWaitingClient:        "等待客户端...",
		StatusStreaming:            "串流中",
		StatusError:                "连接错误",
		StatusSessionEnded:         "会话已结束",
		StatusDisconnected:         "连接已断开",
		LoadingConnectingTitle:     "正在连接到远程桌面...",
		LoadingWaitingTitle:        "等待桌面客户端接入...",
		LoadingErrorTitle:          "连接出现问题",
		LoadingErrorDetail:         "与 Relay 的连接出现错误，请检查网络或稍后重试。",
		LoadingSessionEndedTitle:   "桌面客户端已断开",
		LoadingSessionEndedDetail:  "当前会话已经结束，请重新启动 CLI 后再连接。",
		LoadingDisconnectedTitle:   "连接已断开",
		LoadingDisconnectedDetail:  "正在尝试重新连接远程桌面...",
		SessionDetailPrefix:        "会话 ID：",
		RetryButtonLabel:           "重新连接",
		RetrySessionMessage:        "正在重新连接会话...",
		DefaultDisconnectReason:    "连接已断开",
		DesktopDisconnectedMessage: "桌面客户端已断开，会话已结束。请重新启动 CLI 后再连接。",
		ResolutionLabel:            "分辨率",
		FPSLabel:                   "帧率",
		LatencyLabel:               "延迟",
		SessionLabel:               "会话",
		QualityLabel:               "质量",
		QualityExcellent:           "优秀",
		QualityGood:                "良好",
		QualityFair:                "一般",
		QualityPoor:                "较差",
	}
}
