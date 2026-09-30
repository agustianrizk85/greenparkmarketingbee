#!/usr/bin/env bash
# Deploy backend Marketing (MarketingFlow): tarik kode terbaru, build,
# jalankan/-ulang via PM2. Jalankan di server dari dalam folder repo:
#
#   cd /opt/apps/greenparkmarketingbee && ./deploy.sh
#
# KENAPA BERKAS INI ADA. Repo ini satu-satunya backend yang TIDAK punya
# deploy.sh, dan akibatnya nyata: `deploy-all` menyentuh sembilan repo lain
# dan melewati yang ini diam-diam. Pada 30 Sep 2026 binari yang berjalan
# ternyata dibangun 17 September — 13 hari dan 5 commit tertinggal — sehingga
# rute /api/warroom menjawab 404 di produksi padahal kodenya sudah lama ada di
# GitHub. Tidak ada yang tampak rusak sampai ada yang membuka War Room.
set -euo pipefail
cd "$(dirname "$0")"

echo "==> git pull"
git pull --ff-only

# BEDA DARI REPO LAIN: modul Go-nya ada di backend/, bukan di akar repo.
# `go build ./cmd/server` dari akar akan gagal dengan "no such package".
cd backend

echo "==> go build"
export PATH="$PATH:/usr/local/go/bin"
CGO_ENABLED=0 go build -trimpath -o marketing-server ./cmd/server

# Muat env dari luar git bila ada. Nama berkasnya mengikuti PM2, bukan nama
# repo: prosesnya `marketing-be`, dan repo bernama ...bee/...be sudah cukup
# sering tertukar tanpa perlu ditambah satu nama lagi.
set -a
[ -f /opt/apps/marketing.env ] && . /opt/apps/marketing.env
set +a

# Peringatan, bukan penghalang: backend tetap dijalankan supaya kegagalannya
# terlihat di log PM2 dan bukan di sini.
if [ -z "${APP_PORT:-}" ]; then
  echo "    !! APP_PORT kosong -> backend memakai bawaan kode."
  echo "       Proxy /be/marketing menunjuk :8086, jadi isi di /opt/apps/marketing.env:"
  echo "       APP_PORT=8086"
fi
if [ "${DB_DRIVER:-}" = "sqlite" ]; then
  echo "    !! DB_DRIVER=sqlite -> itu setelan LAPTOP (berkas marketingflow.db)."
  echo "       Di server basisnya Postgres 'greenpark_marketing'. Setel di"
  echo "       /opt/apps/marketing.env: DB_DRIVER=postgres, DB_HOST, DB_PORT,"
  echo "       DB_USER, DB_PASSWORD, DB_NAME=greenpark_marketing"
fi

echo "==> (re)start PM2: marketing-be"
# Path binari ditulis absolut dari folder ini. PM2 menyimpan cwd proses, dan
# `pm2 describe marketing-be` memang menunjukkan cwd .../backend — kalau
# start-nya dijalankan dari akar repo, prosesnya akan mencari berkas relatif
# (UPLOAD_DIR, marketingflow.db) di folder yang salah.
pm2 restart marketing-be --update-env 2>/dev/null \
  || pm2 start "$(pwd)/marketing-server" --name marketing-be --cwd "$(pwd)" --update-env
pm2 save

echo "==> selesai. status:"
pm2 status marketing-be
echo "==> uji rute (401 = ADA tapi minta token; 404 = binari masih basi):"
curl -s -o /dev/null -w "    /api/warroom = %{http_code}\n" "localhost:${APP_PORT:-8086}/api/warroom" || true
