#!/usr/bin/env bash
# XGAME: backup PostgreSQL của XGame AI Proxy, giữ BACKUP_KEEP_DAYS ngày gần nhất.
# Chạy từ bất kỳ đâu: deploy/xgame/backup.sh (cron gợi ý trong README.md).
set -euo pipefail

cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
	echo "backup: thiếu file .env trong $(pwd)" >&2
	exit 1
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

backup_dir="${BACKUP_DIR:-./backups}"
keep_days="${BACKUP_KEEP_DAYS:-7}"
mkdir -p "$backup_dir"

target="$backup_dir/new-api-$(date +%Y%m%d-%H%M%S).dump"
tmp="$target.partial"

docker compose exec -T postgres \
	pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom --no-owner >"$tmp"
mv "$tmp" "$target"
chmod 600 "$target"

find "$backup_dir" -maxdepth 1 -name 'new-api-*.dump' -mtime "+$keep_days" -delete
find "$backup_dir" -maxdepth 1 -name 'new-api-*.dump.partial' -mmin +60 -delete

echo "backup: đã tạo $target"
