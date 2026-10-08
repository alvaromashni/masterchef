# Abre o masterchef com um comando só no Windows: carrega as chaves do .env,
# compila, sobe o painel e abre o navegador. Ctrl+C (ou fechar a janela) encerra.
# Use o masterchef.bat para abrir com duplo clique.
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

function Erro([string]$msg) {
    Write-Host "masterchef: $msg" -ForegroundColor Red
    exit 1
}

if (-not (Test-Path .env)) { Erro 'falta o arquivo .env. Rode: copy .env.example .env e preencha as chaves.' }
if (-not (Test-Path config.yaml)) { Erro 'falta o config.yaml. Rode: copy config.example.yaml config.yaml e ajuste.' }
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Erro 'o Go não está instalado (https://go.dev/dl).' }

# Cada linha NOME=valor do .env vira variável de ambiente deste processo.
foreach ($linha in Get-Content .env) {
    $linha = $linha.Trim()
    if ($linha -eq '' -or $linha.StartsWith('#') -or -not $linha.Contains('=')) { continue }
    $nome, $valor = $linha.Split('=', 2)
    [Environment]::SetEnvironmentVariable($nome.Trim(), $valor.Trim().Trim('"'), 'Process')
}
if (-not $env:LINEAR_API_KEY) { Erro 'LINEAR_API_KEY está vazia no .env.' }
if (-not $env:GITHUB_TOKEN) { Erro 'GITHUB_TOKEN está vazio no .env.' }

$endereco = '127.0.0.1:7777'
$achado = Select-String -Path config.yaml -Pattern '^listen:\s*(\S+)' | Select-Object -First 1
if ($achado) { $endereco = $achado.Matches[0].Groups[1].Value }
$url = "http://$endereco"

function EstaNoAr {
    try { Invoke-WebRequest -Uri "$url/" -UseBasicParsing -TimeoutSec 2 | Out-Null; return $true }
    catch { return $false }
}

function AbrirNavegador {
    try { Start-Process $url } catch { Write-Host "Abra $url no navegador." }
}

# Já está rodando? Só abre o navegador.
if (EstaNoAr) {
    Write-Host "O masterchef já está rodando em $url."
    AbrirNavegador
    exit 0
}

$exe = if ($env:OS -eq 'Windows_NT') { 'painel.exe' } else { 'painel' }
Write-Host 'Compilando...'
go build -o $exe ./cmd/painel
if ($LASTEXITCODE -ne 0) { Erro 'a compilação falhou (veja o erro acima).' }

$proc = Start-Process -FilePath (Join-Path $PSScriptRoot $exe) -ArgumentList '-config', 'config.yaml' -NoNewWindow -PassThru
try {
    for ($i = 0; $i -lt 50; $i++) {
        if (EstaNoAr) { break }
        if ($proc.HasExited) { Erro 'o painel parou ao iniciar (veja o erro acima).' }
        Start-Sleep -Milliseconds 200
    }
    Write-Host "masterchef em $url (Ctrl+C ou feche esta janela para encerrar)."
    AbrirNavegador
    $proc.WaitForExit()
}
finally {
    if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force }
}
