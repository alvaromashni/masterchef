# Painel de Visão de Produto

Painel local e **somente leitura** que mostra o estado real dos produtos a partir do
Linear, do GitHub e dos Markdown do repo central. Veja o [CONTEXT.md](CONTEXT.md) para
a visão completa e as regras do projeto.

> Status: **Fase 6**. O painel coleta issues do Linear e PRs do GitHub, calcula o
> risco de cada PR, vincula issues a PRs, registra o que mudou a cada ciclo e mostra
> a matriz, a fila de review, a tela "o que mudou" e as páginas de escopo e de
> decisões, com o Markdown do repo central. A Fase 6 aplicou o design minimalista
> do Claude Design (Schibsted Grotesk e IBM Plex Mono embutidas no binário, licença
> OFL em `internal/web/static/fontes`). As regras visuais ficam em `/guia`.

## Telas

- **`/` Matriz**: produto × escopo. Cada célula mostra issues em andamento, PRs
  aguardando review (abertos e não rascunho), quantos desses têm risco alto (marca
  vermelha) e há quanto tempo foi a última atividade. Células sem atividade há mais
  que `stale_after` e com issues em andamento ficam hachuradas ("paradas"). A matriz
  se atualiza sozinha a cada 60s.
- **`/mudancas` O que mudou**: mudanças desde a última vez que você clicou em
  "Marcar como visto", agrupadas por produto e escopo. Abrir a página não marca nada
  como visto. A matriz mostra quantas mudanças não vistas cada célula tem.
- **`/prs` Fila de review**: PRs abertos e não rascunho, do maior risco para o
  menor e, no mesmo risco, do mais antigo para o mais novo. Cada PR mostra os
  motivos do risco e as issues vinculadas com os critérios de aceite
  (checkboxes `- [ ]` da descrição da issue). Cada linha abre e fecha; `/prs?abrir=<id>`
  abre um PR específico.
- **`/p/<produto>/<escopo>` Escopo**: aberta ao clicar numa célula da matriz. Mostra
  as issues agrupadas por estado (concluídas e canceladas só dos últimos 7 dias), os PRs abertos com risco e motivos, os últimos 20
  eventos e, ao lado, a seção `## <escopo>` do `PRODUCT.md`. A coluna "a classificar"
  é `/p/<produto>/a-classificar`.
- **`/p/<produto>/decisoes` Decisões**: o `DECISIONS.md` do produto, com um índice.
  Cada seção `## AAAA-MM-DD: título` vira uma decisão.
- **`/guia` Guia de UI**: as regras visuais do painel (cor, tipografia, componentes).

## Repo central (PRODUCT.md e DECISIONS.md)

O `central_repo_path` do `config.yaml` aponta para uma pasta com este formato:

```
painel-central/
├── config.yaml
└── produtos/
    └── <slug>/
        ├── PRODUCT.md     uma seção "## <escopo>" por escopo
        └── DECISIONS.md   texto livre
```

Exemplo de `produtos/produto-x/PRODUCT.md` para um produto com os escopos `api` e `front`:

```md
# Produto X

Texto antes da primeira seção não aparece em nenhum escopo.

## api

O que a API já faz:
- Login com token
- Cadastro de usuários

## front

Telas prontas: login e cadastro. Falta a tela de perfil.
```

O título da seção é comparado sem diferenciar maiúsculas (`## API` vale para o escopo
`api`). A seção vai até o próximo `## ` (títulos dentro de blocos de código não contam).
Os arquivos são lidos a cada abertura da página, então basta salvar e recarregar.
Arquivo ou seção ausente vira um aviso na página, não um erro. HTML escrito direto no
Markdown não é renderizado, por segurança.

## Regras

**Escopo de uma issue** (em qual coluna ela aparece):
1. Se tem PR vinculado, vale o escopo do repo do PR (PRs em dois repos = duas colunas).
2. Senão, a label `scope:<nome>` do Linear.
3. Senão (ou se o escopo não existe no produto), **a classificar**.

**Vínculo issue ↔ PR**: o identificador da issue (ex.: `ABC-123`) aparece na branch,
no título ou no corpo do PR, ou a issue tem no Linear um anexo com a URL do PR.
A branch é comparada sem diferenciar maiúsculas, porque o Linear sugere branches em
minúsculas (`alvaro/abc-123-login`).

**Primeiro sync**: busca todas as issues não canceladas do Linear e, no GitHub, os PRs
abertos mais os fechados/mergeados nos últimos 45 dias (`JanelaPrimeiroSync` em
`internal/collector`). Depois, cada ciclo busca só o que mudou desde o último sync.

**Eventos (o que mudou)**: a cada ciclo, o coletor compara o que veio das APIs com o
banco antes de gravar. Viram eventos: issue nova, issue que mudou de estado, PR aberto,
reaberto, mergeado, fechado sem merge ou atualizado (novo push). O primeiro sync não
gera eventos.

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

Uma vez só, guarde as chaves num `.env` (fica fora do git):

```sh
cp .env.example .env   # e preencha LINEAR_API_KEY e GITHUB_TOKEN
```

Depois, todo dia, um comando só:

```sh
./masterchef.sh
```

O script carrega o `.env`, compila, sobe o painel e abre o navegador. Se o painel
já estiver rodando, ele só abre o navegador. Ctrl+C (ou fechar o terminal) encerra.

- **Rodar de qualquer pasta:** adicione ao `~/.zshrc` (ou `~/.bashrc`)
  `alias masterchef="$HOME/dev/masterchef/masterchef.sh"` e digite `masterchef`.
- **Botão no macOS:** dê duplo clique em `masterchef.command` no Finder (ou arraste
  para o Dock). Na primeira vez, o macOS pode pedir para liberar em *Ajustes → Privacidade
  e Segurança*.

Sem o script, o equivalente manual é:

```sh
export LINEAR_API_KEY=lin_api_...
export GITHUB_TOKEN=github_pat_...
go run ./cmd/painel -config config.yaml
```

## Desenvolvimento

```sh
go vet ./...
go test ./...
gofmt -l .     # não deve listar nada
```
