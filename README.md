# Painel de Visão de Produto

Painel local e **somente leitura** que mostra o estado real dos produtos a partir do
Linear, do GitHub e dos Markdown do repo central. Veja o [CONTEXT.md](CONTEXT.md) para
a visão completa e as regras do projeto.

> Status: **Fase 0** (esqueleto). Ainda não há coleta; a página inicial é um placeholder.

## Requisitos

- Go 1.26 ou mais recente (`go version`)

## 1. Gerar os tokens

**Linear (`LINEAR_API_KEY`)**
1. No Linear, abra *Settings → Security & access → Personal API keys*.
2. Crie uma chave com acesso de leitura.
3. O painel envia a chave no header `Authorization` sem o prefixo `Bearer`.

**GitHub (`GITHUB_TOKEN`)**
1. Em GitHub, vá em *Settings → Developer settings → Personal access tokens → Fine-grained tokens*.
2. Selecione só os repositórios dos produtos.
3. Permissões: **Contents: Read-only** e **Pull requests: Read-only**. Nada de escrita.

## 2. Criar o `config.yaml`

```sh
cp config.example.yaml config.yaml
```

Edite os produtos, escopos (um repo GitHub por escopo) e as regras de risco. O painel
valida tudo ao iniciar e lista todos os problemas de uma vez, por exemplo:

```
erro: config config.yaml inválida:
produto "produto-x": slug duplicado
risco.regras[2]: nivel "critico" inválido (use "alto" ou "medio")
```

## 3. Rodar

```sh
export LINEAR_API_KEY=lin_api_...
export GITHUB_TOKEN=github_pat_...
go run ./cmd/painel -config config.yaml
```

Abra http://127.0.0.1:7777.

Para gerar um binário único: `go build -o painel ./cmd/painel`.

## Desenvolvimento

```sh
go vet ./...
go test ./...
gofmt -l .     # não deve listar nada
```
