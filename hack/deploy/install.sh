#!/usr/bin/env bash
# InfoSentinel 一键安装
#
# 用法:
#   sudo ./install.sh                     安装到 /usr/local/infosentinel(默认)
#   sudo PREFIX=/opt/infosentinel ./install.sh   自定义安装目录
#   sudo ./install.sh --no-systemd        不注册 systemd 服务
#
# 统一目录布局:
#   $PREFIX/bin/infosentinel          二进制
#   $PREFIX/config/config.yaml        引导配置(database/log;业务配置在 Web 界面管理)
#   $PREFIX/.tests/                   运行时依赖与输出(数据库、日志、dry-run 邮件)
#
# 运行时路径由 config.yaml 决定(默认 .tests/...,相对工作目录;
# systemd 以 $PREFIX 为工作目录,如需自定义请安装后编辑 config/config.yaml)。
set -euo pipefail

PREFIX="${PREFIX:-/usr/local/infosentinel}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# 安装包布局:<root>/{bin,config,scripts};脚本位于 scripts/,包根在上一级
PKG_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
NO_SYSTEMD=0
[ "${1:-}" = "--no-systemd" ] && NO_SYSTEMD=1

if [ "$(id -u)" -ne 0 ]; then
  echo "错误:请使用 root/sudo 运行安装脚本" >&2
  exit 1
fi

echo "==> 安装目录:${PREFIX}"

echo "==> 创建系统用户(infosentinel)与服务目录"
SVC_USER="${SVC_USER:-infosentinel}"
if ! id -u "$SVC_USER" >/dev/null 2>&1; then
  if command -v useradd >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SVC_USER" 2>/dev/null || true
    echo "    已创建系统用户 $SVC_USER"
  fi
fi

mkdir -p "$PREFIX/bin" "$PREFIX/config" "$PREFIX/.tests/data" "$PREFIX/.tests/logs" "$PREFIX/.tests/output"

echo "==> 安装二进制"
install -m 0755 "$PKG_ROOT/bin/infosentinel" "$PREFIX/bin/infosentinel"

echo "==> 写入配置(config/config.yaml;已存在则跳过,不会覆盖你的配置)"
if [ ! -f "$PREFIX/config/config.yaml" ]; then
  # 路径由 config.yaml 决定:模板默认 .tests/...(相对工作目录),
  # 需要自定义数据库/日志位置时,安装后直接编辑该文件
  cp "$PKG_ROOT/config/config.example.yaml" "$PREFIX/config/config.yaml"
  echo "    已生成 $PREFIX/config/config.yaml"
else
  echo "    $PREFIX/config/config.yaml 已存在,保持不变"
fi

# 属主:服务以专用用户运行,可写目录归该用户
if id -u "$SVC_USER" >/dev/null 2>&1; then
  chown -R "$SVC_USER:$SVC_USER" "$PREFIX/.tests"
  chown "$SVC_USER:$SVC_USER" "$PREFIX/config" "$PREFIX/config/config.yaml" 2>/dev/null || true
  echo "==> 已设置属主:$SVC_USER"
fi

# systemd 服务
if [ "$NO_SYSTEMD" -eq 0 ] && command -v systemctl >/dev/null 2>&1; then
  echo "==> 注册 systemd 服务(infosentinel.service)"
  sed -e "s|__PREFIX__|${PREFIX}|g" "$SCRIPT_DIR/infosentinel.service" \
    > /etc/systemd/system/infosentinel.service
  systemctl daemon-reload
  systemctl enable infosentinel.service >/dev/null
  echo "    已启用开机自启;启动服务:sudo systemctl start infosentinel"
else
  echo "==> 跳过 systemd(未检测到或使用 --no-systemd)"
  echo "    手动启动:sudo ${PREFIX}/bin/infosentinel -c ${PREFIX}/config/config.yaml serve"
fi

cat <<EOF

安装完成!
  目录:    ${PREFIX}
  启动:    sudo systemctl start infosentinel
  状态:    sudo systemctl status infosentinel
  界面:    http://127.0.0.1:8080(server.addr 可在 Web 界面"全局设置"修改)
  日志:    journalctl -u infosentinel -f  或  ${PREFIX}/.tests/logs/infosentinel.log

下一步:
  1. 按需编辑 ${PREFIX}/config/config.yaml(数据库/日志路径,默认 .tests/ 即可)
  2. export DEEPSEEK_API_KEY=... 并写入 systemd 环境文件(见 ${PREFIX}/config)
  3. 打开管理界面 → 配置页,完成主题 / 推送目标 / 任务设置
EOF
