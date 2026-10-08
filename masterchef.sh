#!/usr/bin/env bash
# Abre o masterchef com um comando só: carrega as chaves do .env, compila,
# sobe o painel e abre o navegador. Ctrl+C (ou fechar o terminal) encerra.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

erro() { echo "masterchef: $*" >&2; exit 1; }

[ -f .env ] || erro "falta o arquivo .env. Rode: cp .env.example .env e preencha as chaves."
[ -f config.yaml ] || erro "falta o config.yaml. Rode: cp config.example.yaml config.yaml e ajuste."
command -v go >/dev/null || erro "o Go não está instalado (https://go.dev/dl)."

set -a
# shellcheck disable=SC1091
. ./.env
set +a
[ -n "${LINEAR_API_KEY:-}" ] || erro "LINEAR_API_KEY está vazia no .env."
[ -n "${GITHUB_TOKEN:-}" ] || erro "GITHUB_TOKEN está vazio no .env."

endereco=$(awk '/^listen:/ {print $2}' config.yaml)
url="http://${endereco:-127.0.0.1:7777}"

abrir() {
  if command -v open >/dev/null; then open "$url"
  elif command -v xdg-open >/dev/null; then xdg-open "$url" >/dev/null 2>&1
  else echo "Abra $url no navegador."
  fi
}

# Já está rodando? Só abre o navegador.
if curl -fsS -o /dev/null "$url/" 2>/dev/null; then
  echo "O masterchef já está rodando em $url."
  abrir
  exit 0
fi

echo "Compilando..."
go build -o painel ./cmd/painel

./painel -config config.yaml &
pid=$!
trap 'kill "$pid" 2>/dev/null' EXIT INT TERM

for _ in $(seq 1 50); do
  curl -fsS -o /dev/null "$url/" 2>/dev/null && break
  kill -0 "$pid" 2>/dev/null || erro "o painel parou ao iniciar (veja o erro acima)."
  sleep 0.2
done

echo "masterchef em $url (Ctrl+C para encerrar)."
abrir
wait "$pid"
