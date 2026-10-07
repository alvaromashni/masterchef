# Painel de Visão de Produto

Painel local e **somente leitura** que mostra o estado real dos produtos a partir do
Linear, do GitHub e dos Markdown do repo central. Veja o [CONTEXT.md](CONTEXT.md) para
a visão completa e as regras do projeto.

> Status: **Fase 2**. O painel coleta issues do Linear e PRs do GitHub, calcula o
> risco de cada PR, vincula issues a PRs e mostra a matriz e a fila de review.

## Telas

- **`/` Matriz**: produto × escopo. Cada célula mostra issues em andamento, PRs
  aguardando review (abertos e não rascunho), um ▲ se algum desses PRs tem risco
  alto e há quanto tempo foi a última atividade. Células sem atividade há mais que
  `stale_after` e com issues em andamento ficam amarelas ("paradas").
- **`/prs` Fila de review**: PRs abertos e não rascunho, do maior risco para o
  menor e, no mesmo risco, do mais antigo para o mais novo. Cada PR mostra os
  motivos do risco e as issues vinculadas com os critérios de aceite
  (checkboxes `- [ ]` da descrição da issue).

## Regras

**Escopo de uma issue** (em qual coluna ela aparece):
1. Se tem PR vinculado, vale o escopo do repo do PR (PRs em dois repos = duas colunas).
2. Senão, a label `scope:<nome>` do Linear.
3. Senão (ou se o escopo não existe no produto), **a classificar**.

**Vínculo issue ↔ PR**: o identificador da issue (ex.: `ABC-123`) aparece na branch,
no título ou no corpo do PR, ou a issue tem no Linear um anexo com a URL do PR.
A branch é comparada sem diferenciar maiúsculas, porque o Linear sugere branches em
minúsculas (`alvaro/abc-123-login`).

**Risco de um PR**: `alto` se algum arquivo casa com uma regra alta; senão `medio` se
casa com uma regra média, se o diff passa de `limite_linhas_diff` ou se nenhum
arquivo casa com `padroes_de_teste`; senão `baixo`. A tela sempre mostra os motivos.

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
