#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
DEFAULT_REPOSITORY="topcheer/deskgo"
LAUNCH_AGENT_LABEL="com.topcheer.deskgo.desktop"
SYSTEMD_SERVICE_NAME="deskgo-desktop.service"

ACTION=""
RELAY_SERVER=""
CODEC=""
SESSION_NAME=""
VERSION="latest"
REPOSITORY="$DEFAULT_REPOSITORY"
AUTOSTART_MODE=""
NON_INTERACTIVE=0
DRY_RUN=0

OS_NAME=""
ARCH_NAME=""
ARTIFACT_NAME=""
INSTALL_ROOT=""
BIN_DIR=""
CONFIG_DIR=""
LOG_DIR=""
BINARY_PATH=""
CONFIG_PATH=""
LAUNCHER_PATH=""
MODE=""
LAUNCH_AGENT_PATH=""
SYSTEMD_SERVICE_PATH=""
XDG_AUTOSTART_PATH=""

usage() {
  cat <<USAGE
DeskGo desktop autostart installer

Usage:
  $SCRIPT_NAME [install|uninstall] [options]

Options:
  --relay-server URL     Relay URL or base site URL (default: DeskGo public relay)
  --codec CODEC          Codec to pin in the service config (jpeg or h264)
  --session NAME         Fixed session name for autostart mode
  --version TAG          Release tag to install (default: latest)
  --repository OWNER/REPO
                         GitHub repository used for downloads (default: $DEFAULT_REPOSITORY)
  --autostart-mode MODE  Linux only: xdg-autostart or systemd-user
  --non-interactive      Disable guided prompts and use defaults for missing values
  --dry-run              Print the planned actions without changing the system
  -h, --help             Show this help

Examples:
  $SCRIPT_NAME install
  $SCRIPT_NAME install --relay-server https://deskgo.example.com --codec h264 --session office-mac --non-interactive
  $SCRIPT_NAME uninstall --non-interactive
USAGE
}

info() {
  printf 'ℹ️  %s\n' "$*"
}

success() {
  printf '✅ %s\n' "$*"
}

warn() {
  printf '⚠️  %s\n' "$*" >&2
}

fatal() {
  printf '❌ %s\n' "$*" >&2
  exit 1
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

run_cmd() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '[dry-run] %s\n' "$*"
    return 0
  fi
  "$@"
}

write_text_file() {
  local path="$1"
  local mode="$2"
  local content="$3"

  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "将写入文件: $path"
    return 0
  fi

  mkdir -p "$(dirname "$path")"
  printf '%s' "$content" > "$path"
  chmod "$mode" "$path"
}

remove_path() {
  local path="$1"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "将删除: $path"
    return 0
  fi
  rm -rf "$path"
}

prompt_default() {
  local message="$1"
  local default_value="$2"
  local answer
  read -r -p "$message [$default_value]: " answer
  answer="$(trim "$answer")"
  if [[ -z "$answer" ]]; then
    printf '%s' "$default_value"
    return
  fi
  printf '%s' "$answer"
}

confirm_action() {
  local message="$1"
  local answer
  while true; do
    read -r -p "$message [Y/n]: " answer
    answer="$(trim "$answer")"
    case "${answer,,}" in
      ""|y|yes)
        return 0
        ;;
      n|no)
        return 1
        ;;
      *)
        warn "请输入 y 或 n"
        ;;
    esac
  done
}

sanitize_session_name() {
  local raw="$1"
  local sanitized
  sanitized="$(printf '%s' "$raw" | tr -c 'A-Za-z0-9._-' '-' | sed -e 's/^-*//' -e 's/-*$//' -e 's/--*/-/g')"
  printf '%s' "$sanitized"
}

json_escape() {
  local value="$1"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '%s' "$value"
}

xml_escape() {
  local value="$1"
  value=${value//&/&amp;}
  value=${value//</&lt;}
  value=${value//>/&gt;}
  value=${value//\"/&quot;}
  printf '%s' "$value"
}

normalize_version() {
  local value="$1"
  if [[ "$value" == "latest" ]]; then
    printf '%s' "$value"
    return
  fi
  if [[ "$value" == v* ]]; then
    printf '%s' "$value"
    return
  fi
  printf 'v%s' "$value"
}

resolve_release_tag() {
  local requested="$1"
  requested="$(normalize_version "$requested")"
  if [[ "$requested" != "latest" ]]; then
    printf '%s' "$requested"
    return
  fi

  info "正在解析 ${REPOSITORY} 的 latest release..." >&2
  local latest_url
  latest_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPOSITORY}/releases/latest")"
  local tag
  tag="${latest_url##*/}"
  [[ -n "$tag" ]] || fatal "无法解析 latest release 标签"
  printf '%s' "$tag"
}

hash_file_sha256() {
  local path="$1"
  if command_exists sha256sum; then
    sha256sum "$path" | awk '{print $1}'
    return
  fi
  if command_exists shasum; then
    shasum -a 256 "$path" | awk '{print $1}'
    return
  fi
  fatal "未找到 sha256sum 或 shasum，无法校验下载文件"
}

normalize_relay_server_url() {
  local input
  input="$(trim "$1")"
  [[ -n "$input" ]] || fatal "Relay 地址不能为空"
  input="${input%/}"

  case "$input" in
    ws://*|wss://*)
      if [[ "$input" == */api/desktop ]]; then
        printf '%s\n' "$input"
      else
        printf '%s/api/desktop\n' "$input"
      fi
      ;;
    http://*)
      input="${input#http://}"
      if [[ "$input" == */api/desktop ]]; then
        printf 'ws://%s\n' "$input"
      else
        printf 'ws://%s/api/desktop\n' "$input"
      fi
      ;;
    https://*)
      input="${input#https://}"
      if [[ "$input" == */api/desktop ]]; then
        printf 'wss://%s\n' "$input"
      else
        printf 'wss://%s/api/desktop\n' "$input"
      fi
      ;;
    *)
      printf 'wss://%s/api/desktop\n' "$input"
      ;;
  esac
}

default_session_name() {
  local host_name
  host_name="$(hostname 2>/dev/null || uname -n 2>/dev/null || printf 'deskgo-host')"
  host_name="$(sanitize_session_name "$host_name")"
  if [[ -z "$host_name" ]]; then
    host_name="deskgo-host"
  fi
  printf '%s' "$host_name"
}

detect_platform() {
  local uname_os uname_arch
  uname_os="$(uname -s)"
  uname_arch="$(uname -m)"

  case "$uname_os" in
    Darwin)
      OS_NAME="darwin"
      ;;
    Linux)
      OS_NAME="linux"
      ;;
    *)
      fatal "当前 shell 脚本仅支持 macOS 和 Linux，请在 Windows 上使用 scripts/deskgo-autostart.ps1"
      ;;
  esac

  case "$uname_arch" in
    x86_64|amd64)
      ARCH_NAME="amd64"
      ;;
    arm64|aarch64)
      ARCH_NAME="arm64"
      ;;
    armv7l|armv7)
      ARCH_NAME="armv7"
      ;;
    riscv64)
      ARCH_NAME="riscv64"
      ;;
    *)
      fatal "不支持的 CPU 架构: $uname_arch"
      ;;
  esac

  if [[ "$OS_NAME" == "darwin" && "$ARCH_NAME" != "amd64" && "$ARCH_NAME" != "arm64" ]]; then
    fatal "当前 release 仅提供 macOS amd64/arm64 Desktop CLI"
  fi

  ARTIFACT_NAME="deskgo-desktop-${OS_NAME}-${ARCH_NAME}"
}

default_codec() {
  case "$OS_NAME" in
    darwin|linux)
      printf 'h264'
      ;;
    *)
      printf 'jpeg'
      ;;
  esac
}

validate_codec() {
  local codec_value="$1"
  case "$OS_NAME" in
    darwin|linux)
      case "$codec_value" in
        jpeg|h264)
          ;;
        *)
          fatal "当前平台仅支持 codec=jpeg 或 codec=h264"
          ;;
      esac
      ;;
    *)
      if [[ "$codec_value" != "jpeg" ]]; then
        fatal "当前平台仅支持 JPEG 自动运行模式"
      fi
      ;;
  esac
}

configure_paths() {
  case "$OS_NAME" in
    darwin)
      INSTALL_ROOT="$HOME/Library/Application Support/DeskGo"
      BIN_DIR="$INSTALL_ROOT/bin"
      CONFIG_DIR="$INSTALL_ROOT"
      LOG_DIR="$HOME/Library/Logs/DeskGo"
      BINARY_PATH="$BIN_DIR/deskgo-desktop"
      CONFIG_PATH="$CONFIG_DIR/deskgo.json"
      LAUNCHER_PATH="$INSTALL_ROOT/run-desktop.sh"
      LAUNCH_AGENT_PATH="$HOME/Library/LaunchAgents/${LAUNCH_AGENT_LABEL}.plist"
      MODE="launchagent"
      if [[ -n "$AUTOSTART_MODE" && "$AUTOSTART_MODE" != "$MODE" ]]; then
        fatal "macOS 仅支持 --autostart-mode launchagent"
      fi
      ;;
    linux)
      local config_home data_home state_home
      config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
      data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
      state_home="${XDG_STATE_HOME:-$HOME/.local/state}"
      INSTALL_ROOT="$data_home/deskgo"
      BIN_DIR="$INSTALL_ROOT/bin"
      CONFIG_DIR="$INSTALL_ROOT"
      LOG_DIR="$state_home/deskgo"
      BINARY_PATH="$BIN_DIR/deskgo-desktop"
      CONFIG_PATH="$CONFIG_DIR/deskgo.json"
      LAUNCHER_PATH="$INSTALL_ROOT/run-desktop.sh"
      SYSTEMD_SERVICE_PATH="$config_home/systemd/user/${SYSTEMD_SERVICE_NAME}"
      XDG_AUTOSTART_PATH="$config_home/autostart/deskgo-desktop.desktop"
      MODE="${AUTOSTART_MODE:-xdg-autostart}"
      case "$MODE" in
        xdg-autostart)
          ;;
        systemd-user)
          command_exists systemctl || fatal "systemd-user 模式需要 systemctl --user"
          ;;
        *)
          fatal "Linux 仅支持 --autostart-mode xdg-autostart 或 systemd-user"
          ;;
      esac
      ;;
  esac
}

write_config_file() {
  local config_content
  config_content=$(cat <<JSON
{
  "server": "$(json_escape "$RELAY_SERVER")",
  "session": "$(json_escape "$SESSION_NAME")",
  "codec": "$(json_escape "$CODEC")"
}
JSON
)
  write_text_file "$CONFIG_PATH" 600 "$config_content"
}

write_launcher_file() {
  local q_install_root q_log_dir q_binary q_stdout q_stderr
  printf -v q_install_root '%q' "$INSTALL_ROOT"
  printf -v q_log_dir '%q' "$LOG_DIR"
  printf -v q_binary '%q' "$BINARY_PATH"
  printf -v q_stdout '%q' "$LOG_DIR/desktop.log"
  printf -v q_stderr '%q' "$LOG_DIR/desktop.error.log"

  local launcher_content
  launcher_content=$(cat <<LAUNCHER
#!/usr/bin/env bash
set -euo pipefail

cd $q_install_root
mkdir -p $q_log_dir
exec $q_binary >> $q_stdout 2>> $q_stderr
LAUNCHER
)

  write_text_file "$LAUNCHER_PATH" 755 "$launcher_content"
}

write_launch_agent() {
  local plist_content
  plist_content=$(cat <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${LAUNCH_AGENT_LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>$(xml_escape "$LAUNCHER_PATH")</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>$(xml_escape "$INSTALL_ROOT")</string>
</dict>
</plist>
PLIST
)
  write_text_file "$LAUNCH_AGENT_PATH" 644 "$plist_content"
}

write_systemd_user_unit() {
  local unit_content
  unit_content=$(cat <<UNIT
[Unit]
Description=DeskGo Desktop CLI
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$(printf '%q' "$LAUNCHER_PATH")
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
UNIT
)
  write_text_file "$SYSTEMD_SERVICE_PATH" 644 "$unit_content"
}

write_xdg_autostart_entry() {
  local exec_line
  exec_line="/bin/bash -lc \"exec $(printf '%q' "$LAUNCHER_PATH")\""
  local desktop_content
  desktop_content=$(cat <<DESKTOP
[Desktop Entry]
Type=Application
Version=1.0
Name=DeskGo Desktop CLI
Comment=Start DeskGo Desktop CLI when the desktop session logs in
Exec=${exec_line}
Terminal=false
X-GNOME-Autostart-enabled=true
DESKTOP
)
  write_text_file "$XDG_AUTOSTART_PATH" 644 "$desktop_content"
}

download_binary() {
  local release_tag download_url checksum_url temp_dir temp_binary checksum_file expected actual
  release_tag="$(resolve_release_tag "$VERSION")"
  download_url="https://github.com/${REPOSITORY}/releases/download/${release_tag}/${ARTIFACT_NAME}"
  checksum_url="https://github.com/${REPOSITORY}/releases/download/${release_tag}/SHA256SUMS.txt"

  info "将安装版本: ${release_tag}"
  info "下载产物: ${ARTIFACT_NAME}"
  info "下载地址: ${download_url}"

  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "将校验 SHA256: ${checksum_url}"
    return 0
  fi

  temp_dir="$(mktemp -d)"
  temp_binary="${temp_dir}/${ARTIFACT_NAME}"
  checksum_file="${temp_dir}/SHA256SUMS.txt"

  mkdir -p "$BIN_DIR"
  curl -fsSL "$download_url" -o "$temp_binary"
  curl -fsSL "$checksum_url" -o "$checksum_file"

  expected="$(awk -v target="$ARTIFACT_NAME" '$2 == target { print $1 }' "$checksum_file")"
  [[ -n "$expected" ]] || fatal "SHA256SUMS.txt 中未找到 ${ARTIFACT_NAME}"

  actual="$(hash_file_sha256 "$temp_binary")"
  if [[ "$actual" != "$expected" ]]; then
    rm -rf "$temp_dir"
    fatal "SHA256 校验失败：expected=${expected} actual=${actual}"
  fi

  mv "$temp_binary" "$BINARY_PATH"
  chmod 755 "$BINARY_PATH"
  rm -rf "$temp_dir"
}

stop_mac_launch_agent() {
  local gui_domain
  gui_domain="gui/$(id -u)"
  if command_exists launchctl; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
      info "将停止 LaunchAgent: ${LAUNCH_AGENT_LABEL}"
    else
      launchctl bootout "$gui_domain" "$LAUNCH_AGENT_PATH" >/dev/null 2>&1 || true
    fi
  fi
}

install_mac_launch_agent() {
  stop_mac_launch_agent
  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "将加载 LaunchAgent: ${LAUNCH_AGENT_PATH}"
    return 0
  fi
  launchctl bootstrap "gui/$(id -u)" "$LAUNCH_AGENT_PATH"
  launchctl kickstart -k "gui/$(id -u)/${LAUNCH_AGENT_LABEL}"
}

remove_linux_systemd_registration() {
  if command_exists systemctl; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
      info "将停用 systemd 用户服务: ${SYSTEMD_SERVICE_NAME}"
    else
      systemctl --user disable --now "$SYSTEMD_SERVICE_NAME" >/dev/null 2>&1 || true
      systemctl --user daemon-reload >/dev/null 2>&1 || true
    fi
  fi
  remove_path "$SYSTEMD_SERVICE_PATH"
}

install_linux_systemd_registration() {
  remove_path "$XDG_AUTOSTART_PATH"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    info "将启用 systemd 用户服务: ${SYSTEMD_SERVICE_NAME}"
    return 0
  fi
  systemctl --user daemon-reload
  systemctl --user enable --now "$SYSTEMD_SERVICE_NAME"
}

remove_linux_xdg_registration() {
  remove_path "$XDG_AUTOSTART_PATH"
}

install_linux_xdg_registration() {
  remove_linux_systemd_registration
  info "XDG autostart 模式会在下次图形会话登录时自动启动"
}

interactive_collect_install_options() {
  info "进入 DeskGo 自动运行引导安装"

  RELAY_SERVER="$(prompt_default 'Relay 地址（支持 https://host 或 wss://host/api/desktop）' "${RELAY_SERVER:-wss://deskgo.zty8.cn/api/desktop}")"

  if [[ "$OS_NAME" == "linux" ]]; then
    local default_mode
    default_mode="${AUTOSTART_MODE:-xdg-autostart}"
    while true; do
      AUTOSTART_MODE="$(prompt_default 'Linux 自动运行模式（xdg-autostart 或 systemd-user）' "$default_mode")"
      case "$AUTOSTART_MODE" in
        xdg-autostart|systemd-user)
          break
          ;;
        *)
          warn "请输入 xdg-autostart 或 systemd-user"
          ;;
      esac
    done
  fi

  local default_codec_value
  default_codec_value="${CODEC:-$(default_codec)}"
  while true; do
    CODEC="$(prompt_default 'Codec（jpeg 或 h264）' "$default_codec_value")"
    CODEC="${CODEC,,}"
    if [[ "$OS_NAME" == "darwin" || "$OS_NAME" == "linux" ]]; then
      case "$CODEC" in
        jpeg|h264)
          break
          ;;
      esac
      warn "请输入 jpeg 或 h264"
    fi
  done

  SESSION_NAME="$(prompt_default '固定 Session 名字' "${SESSION_NAME:-$(default_session_name)}")"
  VERSION="$(prompt_default '安装的 release 版本' "$VERSION")"
}

ensure_action() {
  if [[ -n "$ACTION" ]]; then
    return
  fi

  if [[ "$NON_INTERACTIVE" -eq 1 ]]; then
    fatal "非引导模式必须显式指定 install 或 uninstall"
  fi

  while true; do
    local chosen
    chosen="$(prompt_default '请选择操作（install / uninstall）' 'install')"
    chosen="${chosen,,}"
    case "$chosen" in
      install|uninstall)
        ACTION="$chosen"
        return
        ;;
      *)
        warn "请输入 install 或 uninstall"
        ;;
    esac
  done
}

prepare_install_values() {
  RELAY_SERVER="$(normalize_relay_server_url "${RELAY_SERVER:-wss://deskgo.zty8.cn/api/desktop}")"
  CODEC="${CODEC:-$(default_codec)}"
  CODEC="${CODEC,,}"
  validate_codec "$CODEC"

  if [[ -z "$SESSION_NAME" ]]; then
    SESSION_NAME="$(default_session_name)"
  fi
  SESSION_NAME="$(sanitize_session_name "$SESSION_NAME")"
  [[ -n "$SESSION_NAME" ]] || fatal "Session 名字为空，请使用字母、数字、点、下划线或连字符"

  VERSION="$(normalize_version "$VERSION")"
  configure_paths
}

print_install_summary() {
  cat <<SUMMARY

DeskGo 自动运行配置摘要
  平台/架构 : ${OS_NAME}/${ARCH_NAME}
  下载仓库  : ${REPOSITORY}
  Release    : ${VERSION}
  Relay      : ${RELAY_SERVER}
  Codec      : ${CODEC}
  Session    : ${SESSION_NAME}
  模式       : ${MODE}
  二进制路径 : ${BINARY_PATH}
  配置文件   : ${CONFIG_PATH}（通过工作目录兼容旧 release）
  启动脚本   : ${LAUNCHER_PATH}
SUMMARY
  case "$OS_NAME" in
    darwin)
      printf '  LaunchAgent : %s\n' "$LAUNCH_AGENT_PATH"
      ;;
    linux)
      if [[ "$MODE" == "systemd-user" ]]; then
        printf '  systemd 单元: %s\n' "$SYSTEMD_SERVICE_PATH"
      else
        printf '  XDG 自启项  : %s\n' "$XDG_AUTOSTART_PATH"
      fi
      ;;
  esac
  printf '\n'
}

install_action() {
  if [[ "$NON_INTERACTIVE" -eq 0 ]]; then
    interactive_collect_install_options
  fi

  prepare_install_values
  print_install_summary

  if [[ "$NON_INTERACTIVE" -eq 0 ]] && ! confirm_action '确认继续安装吗？'; then
    fatal "已取消安装"
  fi

  download_binary
  write_config_file
  write_launcher_file

  case "$OS_NAME" in
    darwin)
      write_launch_agent
      install_mac_launch_agent
      ;;
    linux)
      case "$MODE" in
        systemd-user)
          write_systemd_user_unit
          install_linux_systemd_registration
          ;;
        xdg-autostart)
          write_xdg_autostart_entry
          install_linux_xdg_registration
          ;;
      esac
      ;;
  esac

  success "DeskGo 自动运行安装完成"
  if [[ "$OS_NAME" == "linux" && "$MODE" == "xdg-autostart" ]]; then
    info "如需立即启动，可执行: ${LAUNCHER_PATH}"
  fi
}

uninstall_action() {
  detect_platform
  configure_paths

  cat <<SUMMARY

DeskGo 自动运行卸载摘要
  平台/架构 : ${OS_NAME}/${ARCH_NAME}
  安装根目录 : ${INSTALL_ROOT}
  配置文件   : ${CONFIG_PATH}
  启动脚本   : ${LAUNCHER_PATH}
SUMMARY
  printf '\n'

  if [[ "$NON_INTERACTIVE" -eq 0 ]] && ! confirm_action '确认卸载当前用户下的 DeskGo 自动运行配置吗？'; then
    fatal "已取消卸载"
  fi

  case "$OS_NAME" in
    darwin)
      stop_mac_launch_agent
      remove_path "$LAUNCH_AGENT_PATH"
      ;;
    linux)
      remove_linux_systemd_registration
      remove_linux_xdg_registration
      ;;
  esac

  remove_path "$INSTALL_ROOT"
  remove_path "$CONFIG_DIR"
  remove_path "$LOG_DIR"

  success "DeskGo 自动运行已卸载"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      install|uninstall)
        [[ -z "$ACTION" ]] || fatal "请只指定一个操作：install 或 uninstall"
        ACTION="$1"
        shift
        ;;
      --relay-server)
        [[ $# -ge 2 ]] || fatal "--relay-server 需要一个值"
        RELAY_SERVER="$2"
        shift 2
        ;;
      --codec)
        [[ $# -ge 2 ]] || fatal "--codec 需要一个值"
        CODEC="$2"
        shift 2
        ;;
      --session)
        [[ $# -ge 2 ]] || fatal "--session 需要一个值"
        SESSION_NAME="$2"
        shift 2
        ;;
      --version)
        [[ $# -ge 2 ]] || fatal "--version 需要一个值"
        VERSION="$2"
        shift 2
        ;;
      --repository)
        [[ $# -ge 2 ]] || fatal "--repository 需要一个值"
        REPOSITORY="$2"
        shift 2
        ;;
      --autostart-mode)
        [[ $# -ge 2 ]] || fatal "--autostart-mode 需要一个值"
        AUTOSTART_MODE="$2"
        shift 2
        ;;
      --non-interactive)
        NON_INTERACTIVE=1
        shift
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        fatal "未知参数: $1"
        ;;
    esac
  done
}

main() {
  parse_args "$@"
  ensure_action
  detect_platform

  case "$ACTION" in
    install)
      install_action
      ;;
    uninstall)
      uninstall_action
      ;;
    *)
      fatal "未知操作: $ACTION"
      ;;
  esac
}

main "$@"
