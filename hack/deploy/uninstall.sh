#!/usr/bin/env bash
# InfoSentinel 一键卸载
#
# 用法:
#   sudo ./uninstall.sh                   卸载 /usr/local/infosentinel(默认)
#   sudo PREFIX=/opt/infosentinel ./uninstall.sh
#   sudo ./uninstall.sh --purge           连同数据(config/.tests)一起删除
#
# 默认保留数据(--purge 才删除),避免误删配置与历史产物。
set -euo pipefail

PREFIX="${PREFIX:-/usr/local/infosentinel}"
PURGE=0
[ "${1:-}" = "--purge" ] && PURGE=1

if [ "$(id -u)" -ne 0 ]; then
  echo "错误:请使用 root/sudo 运行卸载脚本" >&2
  exit 1
fi

echo "==> 停止并移除 systemd 服务"
if command -v systemctl >/dev/null 2>&1; then
  systemctl stop infosentinel.service 2>/dev/null || true
  systemctl disable infosentinel.service 2>/dev/null || true
  rm -f /etc/systemd/system/infosentinel.service
  systemctl daemon-reload
fi

if [ "$PURGE" -eq 1 ]; then
  echo "==> 卸载(--purge):删除整个目录 ${PREFIX}"
  rm -rf "$PREFIX"
  echo "卸载完成,所有数据已删除。"
else
  echo "==> 卸载:删除二进制,保留数据"
  rm -f "$PREFIX/bin/infosentinel"
  rmdir "$PREFIX/bin" 2>/dev/null || true
  cat <<EOF

卸载完成(保留数据)。
  已删除: ${PREFIX}/bin/infosentinel
  已保留: ${PREFIX}/config  ${PREFIX}/.tests
  彻底删除请运行: sudo ./uninstall.sh --purge
EOF
fi
