# CONTEXT.md — Painel de Visão de Produto

> Documento de contexto para agentes de código (Claude Code, Codex etc.).
> Leia este arquivo inteiro antes de qualquer tarefa neste repositório.
> Em caso de conflito entre este arquivo e uma instrução de tarefa, pergunte antes de agir.

---

## 1. O problema

O dono deste projeto atua como **PM e revisor de código** de 2–3 produtos simultâneos. Cada produto tem 3–4 escopos (app, front, api, infra), e **cada escopo é um repositório GitHub separado**. Todo o código é escrito por agentes de IA, orquestrados via Linear + Claude Code. Ele não escreve código, só revisa PRs.

A dor: ele **perde a visão do produto** (o que foi feito, o que falta, o que está parado) e acaba dependendo do agente para saber o estado das coisas. Isso é perigoso, porque o resumo de um agente é uma interpretação, não um fato.

## 2. O que este software é

Um **painel local, somente leitura**, que mostra o estado real de todos os produtos e escopos a partir de **fontes factuais**:

- **Linear**: issues, estados, quando mudaram
- **GitHub**: PRs, arquivos alterados, status de merge
- **Markdown escrito pelo dono**: o que cada escopo já faz e as decisões de produto

Princípio central: **o painel nunca usa IA para resumir ou interpretar estado.** Tudo que aparece na tela é dado bruto ou regra determinística.

## 3. Princípios de projeto (em ordem de prioridade)

1. **Simplicidade acima de tudo.** Um binário, um arquivo SQLite, sem serviços externos próprios.
2. **Somente leitura.** O painel nunca escreve no Linear nem no GitHub.
3. **Determinismo.** Nenhuma chamada a LLM no MVP. Risco e estado vêm de regras e dados.
4. **Biblioteca padrão primeiro.** Só adicione dependência quando a stdlib do Go claramente não resolver.
5. **Código legível para um revisor júnior.** Prefira código óbvio a código esperto. Funções curtas, nomes explícitos, comentários explicando o *porquê*.

## 4. Fora de escopo (não implementar sem pedido explícito)

- Autenticação/login (roda em `localhost`)
- Webhooks (usamos polling)
- Postgres, Redis, filas, Docker
- Framework SPA (React, Vue etc.) ou build de Node
- Escrita no Linear ou no GitHub
- Qualquer chamada a LLM
- Servidor MCP (fase futura, ver seção 13)

## 5. Stack

| Camada | Escolha | Motivo |
|---|---|---|
| Linguagem | Go (versão estável mais recente) | Binário único, concorrência simples |
| HTTP | `net/http` da stdlib (roteamento com padrões de método/path) | Sem framework |
| Templates | `html/template` + `embed` | HTML no servidor, embutido no binário |
| Interatividade | htmx (via CDN) | Sem build de front |
| CSS | Um arquivo CSS próprio, simples | Sem Tailwind/build |
| Banco | SQLite com `modernc.org/sqlite` | Go puro, sem CGO |
| Config | YAML com `gopkg.in/yaml.v3` | Legível e editável à mão |
| Globs | `github.com/bmatcuk/doublestar/v4` | Suporte a `**` nas regras de risco |
| Markdown | `github.com/yuin/goldmark` | Renderizar PRODUCT.md / DECISIONS.md |
| Linear | GraphQL via `net/http` puro | Sem SDK |
| GitHub | REST API via `net/http` puro | Poucos endpoints, sem SDK |

## 6. Arquitetura

```
Linear API ─┐
            ├─► coletor (polling) ─► motor de risco ─► SQLite ─► servidor web ─► navegador
GitHub API ─┘         ▲                                              ▲
                      │                                              │
            config.yaml (repo central)              PRODUCT.md / DECISIONS.md (repo central)
```

Tudo roda num único processo:

- **Coletor**: goroutine com `time.Ticker`, roda a cada `poll_interval`. Também roda uma vez na inicialização.
- **Motor de risco**: função pura que recebe a lista de arquivos de um PR e as regras e devolve nível + motivos.
- **Store**: camada fina sobre SQLite. Toda SQL fica aqui.
- **Web**: handlers HTTP que leem do store e renderizam templates.

### Estrutura de pastas

```
cmd/painel/main.go          # wiring: config, store, coletor, servidor
internal/config/            # carregar e validar config.yaml
internal/linear/            # cliente GraphQL do Linear
internal/github/            # cliente REST do GitHub
internal/collector/         # orquestra a coleta e gera eventos
internal/risk/              # motor de risco (função pura)
internal/store/             # SQLite: migrações e queries
internal/store/migrations/  # arquivos .sql embutidos
internal/product/           # leitura e render de PRODUCT.md / DECISIONS.md
internal/web/               # handlers
internal/web/templates/     # .html embutidos
internal/web/static/        # CSS embutido
```

Os clientes `linear` e `github` devem ser expostos por **interfaces** consumidas pelo `collector`, para que os testes usem implementações falsas sem rede.

## 7. Configuração

### Segredos (variáveis de ambiente, nunca no YAML)

- `LINEAR_API_KEY`: chave pessoal do Linear (enviada no header `Authorization`, sem prefixo `Bearer`)
- `GITHUB_TOKEN`: token fine-grained **somente leitura** (Contents e Pull requests: read)

### `config.yaml` (fica no repo central)

```yaml
poll_interval: 5m
listen: 127.0.0.1:7777
db_path: ./painel.db
central_repo_path: ~/dev/painel-central   # onde estão PRODUCT.md e DECISIONS.md

stale_after: 72h          # célula sem atividade há mais que isso fica "parada"

produtos:
  - nome: Produto X
    slug: produto-x
    linear_project_id: "uuid-do-projeto-no-linear"
    escopos:
      api: alvaromashni/produto-x-api
      front: alvaromashni/produto-x-front
      infra: alvaromashni/produto-x-infra

risco:
  limite_linhas_diff: 400
  regras:                       # aplicadas a todos os repos
    - padrao: "**/migrations/**"
      nivel: alto
      motivo: "Altera migração de banco"
    - padrao: "**/auth/**"
      nivel: alto
      motivo: "Mexe em autenticação/permissões"
    - padrao: "**/Dockerfile"
      nivel: medio
      motivo: "Altera imagem de container"
    - padrao: ".github/workflows/**"
      nivel: medio
      motivo: "Altera CI/CD"
  padroes_de_teste:             # se nenhum arquivo do PR casar, alerta "sem testes"
    - "**/*_test.go"
    - "**/*.test.*"
    - "**/*.spec.*"
    - "**/src/test/**"
```

A validação da config deve falhar cedo, com mensagem clara: slug duplicado, repo duplicado entre escopos, nível de risco inválido etc.

## 8. Regras de domínio

### Escopo de uma issue

1. Se a issue tem PR vinculado → o escopo é o do repo do PR (via `config.yaml`).
2. Senão, se a issue tem label `scope:<nome>` → usa a label.
3. Senão → a issue vai para a linha **"a classificar"** do produto.

Se uma issue tiver PRs em repos de escopos diferentes, ela aparece em todas as células correspondentes.

### Vínculo issue ↔ PR

Um PR está vinculado a uma issue se o **identificador da issue** (ex.: `ABC-123`, regex `[A-Z][A-Z0-9]+-\d+`) aparece no **nome da branch**, no **título** ou no **corpo** do PR. Também considerar os anexos da issue no Linear que apontem para a URL do PR. O vínculo é calculado pelo coletor e gravado no banco.

### Nível de risco de um PR

- `alto` se qualquer arquivo casa com uma regra `alto`
- senão `medio` se casa com regra `medio`, ou se o diff passa de `limite_linhas_diff`, ou se nenhum arquivo casa com `padroes_de_teste`
- senão `baixo`

O resultado sempre inclui a **lista de motivos**, nunca só o nível.

### Célula "parada"

Uma célula (produto × escopo) está parada se a última atividade (mudança de issue ou de PR) é mais antiga que `stale_after` **e** ela tem issues em andamento.

## 9. Banco de dados (SQLite)

Migrações em arquivos `.sql` numerados, embutidos e aplicados na inicialização (tabela `schema_migrations`).

```sql
CREATE TABLE issues (
  id            TEXT PRIMARY KEY,      -- id do Linear
  identifier    TEXT NOT NULL,         -- ex.: ABC-123
  product_slug  TEXT NOT NULL,
  title         TEXT NOT NULL,
  state_name    TEXT NOT NULL,
  state_type    TEXT NOT NULL,         -- backlog, unstarted, started, completed, canceled
  scope_label   TEXT,                  -- de scope:<nome>, se houver
  url           TEXT NOT NULL,
  description   TEXT,
  updated_at    TEXT NOT NULL          -- ISO 8601
);

CREATE TABLE pull_requests (
  id            INTEGER PRIMARY KEY,   -- id do GitHub
  repo          TEXT NOT NULL,         -- owner/name
  number        INTEGER NOT NULL,
  product_slug  TEXT NOT NULL,
  scope         TEXT NOT NULL,
  title         TEXT NOT NULL,
  branch        TEXT NOT NULL,
  state         TEXT NOT NULL,         -- open, closed, merged
  draft         INTEGER NOT NULL,
  additions     INTEGER NOT NULL,
  deletions     INTEGER NOT NULL,
  risk_level    TEXT NOT NULL,
  risk_reasons  TEXT NOT NULL,         -- JSON array de strings
  url           TEXT NOT NULL,
  updated_at    TEXT NOT NULL,
  UNIQUE (repo, number)
);

CREATE TABLE issue_prs (
  issue_id  TEXT NOT NULL,
  pr_id     INTEGER NOT NULL,
  PRIMARY KEY (issue_id, pr_id)
);

CREATE TABLE events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  occurred_at  TEXT NOT NULL,          -- quando o coletor detectou
  product_slug TEXT NOT NULL,
  scope        TEXT,                   -- nulo = "a classificar"
  kind         TEXT NOT NULL,          -- issue_created, issue_state_changed, pr_opened, pr_merged, pr_closed, pr_updated
  ref          TEXT NOT NULL,          -- ABC-123 ou owner/repo#42
  summary      TEXT NOT NULL,          -- ex.: "In Progress → In Review"
  url          TEXT NOT NULL
);

CREATE TABLE meta (
  key    TEXT PRIMARY KEY,             -- last_visit, last_sync_ok, last_sync_error
  value  TEXT NOT NULL
);
```

### Como os eventos nascem

O coletor compara o que veio da API com o que está no banco **antes** de gravar. Cada diferença relevante (issue nova, mudança de estado, PR aberto/mergeado/fechado) vira uma linha em `events`. A tela "o que mudou" é simplesmente `SELECT * FROM events WHERE occurred_at > last_visit`.

Na **primeira sincronização** (banco vazio), não gerar eventos: apenas popular as tabelas.

## 10. Coleta

A cada ciclo:

1. Para cada produto, buscar no Linear as issues do projeto atualizadas desde o último sync bem-sucedido (no primeiro sync, todas as não canceladas). Paginar.
2. Para cada repo configurado, buscar no GitHub os PRs abertos e os fechados/mergeados atualizados desde o último sync. Para PRs novos ou alterados, buscar a lista de arquivos (paginada) e calcular o risco.
3. Calcular vínculos issue ↔ PR.
4. Gerar eventos, gravar tudo **numa transação**.
5. Atualizar `last_sync_ok`. Em erro, gravar `last_sync_error` com a mensagem e **não** atualizar `last_sync_ok`.

Erros de um repo/produto não podem derrubar o ciclo inteiro: registre e continue. Respeite rate limits (cabeçalhos do GitHub) com espera simples; não implemente retry sofisticado.

## 11. Telas

Todas renderizadas no servidor. O topo de toda página mostra a hora do último sync e um aviso visível se o último sync falhou.

1. **`/` — Matriz.** Linhas = produtos, colunas = escopos (+ coluna "a classificar"). Cada célula mostra: issues em andamento, PRs aguardando review, indicador de risco alto pendente, tempo desde a última atividade e destaque se estiver parada. Clicar leva à página do escopo.
2. **`/mudancas` — O que mudou.** Eventos desde a última visita, agrupados por produto e escopo. Botão "marcar como visto" (POST via htmx) atualiza `last_visit`. **Abrir a página não atualiza `last_visit` sozinho.**
3. **`/p/{produto}/{escopo}` — Escopo.** Issues por estado, PRs abertos com nível e motivos de risco, eventos recentes desse escopo e a seção correspondente do `PRODUCT.md`.
4. **`/prs` — Fila de review.** Todos os PRs abertos não-draft, ordenados por risco (alto primeiro) e depois por idade. Cada item mostra a issue vinculada e seus critérios de aceite (checkboxes `- [ ]` extraídos da descrição da issue, apenas exibidos).
5. **`/p/{produto}/decisoes` — Decisões.** `DECISIONS.md` renderizado.

### Repo central: formato dos Markdown

```
painel-central/
  config.yaml
  produtos/
    produto-x/
      PRODUCT.md      # uma seção "## <escopo>" por escopo
      DECISIONS.md
```

Se um arquivo não existir, a tela mostra um aviso amigável, nunca um erro.

## 12. Qualidade

- `go vet` e `gofmt` sem avisos.
- Testes unitários obrigatórios para: motor de risco, regra de escopo, vínculo issue ↔ PR, geração de eventos (diff), validação de config.
- Testes **não acessam rede**. Use as interfaces dos clientes com implementações falsas e fixtures JSON em `testdata/`.
- Erros sempre com contexto (`fmt.Errorf("buscando PRs de %s: %w", repo, err)`).
- Logs com `log/slog`.
- README com: como gerar os tokens, como criar o `config.yaml` e como rodar.

## 13. Roadmap

| Fase | Entrega |
|---|---|
| 0 | Esqueleto: config, store com migrações, servidor com página vazia |
| 1 | Coletor do Linear + matriz (só issues) |
| 2 | Coletor do GitHub + motor de risco + vínculo issue ↔ PR + fila de review |
| 3 | Eventos + tela "o que mudou" |
| 4 | Páginas de escopo e decisões com Markdown do repo central |
| Futuro | Servidor MCP no mesmo binário (SDK oficial de MCP para Go): tools somente leitura do estado + gravação de log estruturado do agente |
| Futuro | Revisor independente com outro modelo (opcional, sempre rotulado como opinião) |

## 14. Regras para agentes trabalhando neste repo

- **Uma fase por PR.** Não adiante trabalho de fases futuras.
- PRs pequenos e com descrição contendo: o que foi feito, como testar manualmente, decisões tomadas e dúvidas em aberto.
- Se algo neste documento parecer errado ou ambíguo, **pare e pergunte** em vez de assumir.
- Nunca adicione dependência fora da tabela da seção 5 sem justificar na descrição do PR.
- Nunca mova issues para "Done". O dono do projeto fecha as issues após revisar.
