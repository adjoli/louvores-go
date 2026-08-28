#!/usr/bin/env bash
# Gera o CSS do Tailwind para a interface web.
# Uso: ./scripts/build-css.sh
#
# O node/npm estão instalados no Windows, mas o projeto roda no WSL. Como o
# node.exe é um binário Windows, o processo precisa de um CWD em caminho
# Windows real (sem UNC). Por isso o build roda num diretório temporário do
# Windows (sem espaços), para onde copiamos o input e os templates, e o
# main.css resultante é copiado de volta para internal/web/static/.
set -euo pipefail

NODE_BIN="${NODE_BIN:-}"
if [ -z "$NODE_BIN" ]; then
  if [ -x "/mnt/c/Programas/nodejs/24.11/node.exe" ]; then
    NODE_BIN="/mnt/c/Programas/nodejs/24.11/node.exe"
  else
    NODE_BIN="/mnt/c/Program Files/nodejs/24.11/node.exe"
  fi
fi
echo "node: $NODE_BIN"

NODE_WIN="$(wslpath -m "$NODE_BIN")"
NPM_CLI_WIN="${NODE_WIN%/node.exe}/node_modules/npm/bin/npx-cli.js"
echo "npx: $NPM_CLI_WIN"

# Diretório temporário no Windows (sem espaços) para servir de CWD do build.
# O opencode temp é em C:\Users\N2SE\AppData\Local\Temp\opencode.
STAGE="/mnt/c/Users/N2SE/AppData/Local/Temp/opencode/tailwind-louvores"
rm -rf "$STAGE"
mkdir -p "$STAGE/templates" "$STAGE/static"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cp "$ROOT/internal/web/static/input.css" "$STAGE/static/input.css"
cp "$ROOT"/internal/web/templates/*.templ "$STAGE/templates/"

cd "$STAGE"
"$NODE_BIN" "$NPM_CLI_WIN" --yes tailwindcss@3.4.17 \
  -i static/input.css \
  -o static/main.css \
  --content "templates/*.templ"

# Copia o CSS gerado de volta para o projeto.
mkdir -p "$ROOT/internal/web/static"
cp "$STAGE/static/main.css" "$ROOT/internal/web/static/main.css"
echo "main.css copiado para internal/web/static/main.css"
