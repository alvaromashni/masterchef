# Prompts de implementação — Painel de Visão de Produto

Use **um prompt por sessão do Claude Code**, na ordem. Só passe para o próximo depois de revisar e mergear o PR da fase anterior.

Antes da Fase 0: crie o repositório vazio, coloque o `CONTEXT.md` na raiz e faça o primeiro commit.

---

## Fase 0 — Esqueleto

```
Leia o CONTEXT.md inteiro antes de começar.

Implemente apenas a Fase 0 do roadmap (seção 13):

- go.mod e estrutura de pastas da seção 6
- internal/config: carregar config.yaml (caminho via flag -config), expandir ~ nos caminhos, ler LINEAR_API_KEY e GITHUB_TOKEN do ambiente e validar tudo conforme a seção 7. Erros claros.
- internal/store: abrir SQLite com modernc.org/sqlite, sistema de migrações com arquivos .sql embutidos e tabela schema_migrations, e a migração inicial com o schema completo da seção 9
- internal/web: servidor net/http em config.listen, layout base com htmx via CDN, CSS simples embutido e uma página "/" com um placeholder
- cmd/painel/main.go fazendo o wiring
- Um config.example.yaml e um README inicial
- Testes da validação de config

Não implemente coleta, Linear nem GitHub ainda.

Ao terminar: rode go vet e go test ./..., abra um PR com a descrição pedida na seção 14 e liste qualquer dúvida sobre o CONTEXT.md.
```

---

## Fase 1 — Linear + matriz

```
Leia o CONTEXT.md. A Fase 0 já está mergeada.

Implemente apenas a Fase 1:

- internal/linear: cliente GraphQL com net/http puro, atrás de uma interface. Buscar issues de um projeto (por linear_project_id) com paginação e filtro opcional de updatedAt, trazendo id, identifier, título, descrição, estado (nome e tipo), labels, URL, updatedAt e anexos.
- internal/collector: goroutine com ticker (poll_interval) que roda também na inicialização, busca as issues de cada produto e grava no store numa transação. Atualiza last_sync_ok / last_sync_error na tabela meta. Erro em um produto não derruba os outros.
- Regra de escopo da seção 8 usando só a label scope:<nome> por enquanto (o GitHub entra na Fase 2). Issues sem label vão para "a classificar".
- Página "/" com a matriz da seção 11, mostrando por enquanto apenas a contagem de issues em andamento por célula e o aviso de último sync no topo.
- Testes com cliente falso e fixtures em testdata/, sem rede.

Não gere eventos ainda (Fase 3). Ao terminar: go vet, go test ./..., PR com a descrição da seção 14.
```

---

## Fase 2 — GitHub, risco e fila de review

```
Leia o CONTEXT.md. As fases 0 e 1 estão mergeadas.

Implemente apenas a Fase 2:

- internal/github: cliente REST com net/http puro, atrás de interface. Listar PRs (abertos e fechados/mergeados atualizados desde uma data), com paginação, e listar arquivos de um PR. Respeitar rate limit de forma simples.
- internal/risk: função pura que recebe arquivos, total de linhas alteradas e a config de risco e devolve nível e motivos, conforme a seção 8. Testes de tabela cobrindo cada regra.
- Vínculo issue ↔ PR conforme a seção 8 (identificador na branch, título ou corpo, mais anexos do Linear). Testes.
- Atualizar a regra de escopo: PR vinculado tem prioridade sobre a label.
- Coletor passa a buscar PRs de todos os repos configurados e calcular risco só para PRs novos ou alterados.
- Matriz ganha: PRs aguardando review, indicador de risco alto pendente, tempo desde a última atividade e destaque de célula parada (stale_after).
- Página "/prs" conforme a seção 11, incluindo a extração dos checkboxes "- [ ]" da descrição da issue vinculada.

Não gere eventos ainda. Ao terminar: go vet, go test ./..., PR com a descrição da seção 14.
```

---

## Fase 3 — O que mudou

```
Leia o CONTEXT.md. As fases 0 a 2 estão mergeadas.

Implemente apenas a Fase 3:

- No coletor, antes de gravar, comparar o estado vindo das APIs com o do banco e gerar eventos conforme a seção 9 ("Como os eventos nascem"). No primeiro sync (banco vazio), não gerar eventos.
- A geração de eventos deve ser uma função pura (estado antigo + estado novo → lista de eventos), com testes de tabela cobrindo cada tipo de evento.
- Página "/mudancas" agrupada por produto e escopo, com o botão "marcar como visto" via htmx (POST) atualizando last_visit. Abrir a página NÃO atualiza last_visit.
- Na matriz, mostrar um contador de mudanças não vistas por célula.

Ao terminar: go vet, go test ./..., PR com a descrição da seção 14.
```

---

## Fase 4 — Páginas de escopo e decisões

```
Leia o CONTEXT.md. As fases 0 a 3 estão mergeadas.

Implemente apenas a Fase 4:

- internal/product: ler produtos/<slug>/PRODUCT.md e DECISIONS.md de central_repo_path, extrair a seção "## <escopo>" do PRODUCT.md e renderizar com goldmark. Arquivo ausente gera aviso amigável, nunca erro. Testes.
- Página "/p/{produto}/{escopo}" conforme a seção 11.
- Página "/p/{produto}/decisoes".
- Links da matriz e da fila de review apontando para essas páginas.
- Atualizar o README com o formato esperado do repo central e um exemplo de PRODUCT.md.

Ao terminar: go vet, go test ./..., PR com a descrição da seção 14.
```

---

## Fase 5 — Refinamento de UI

```
Leia o CONTEXT.md. As fases 0 a 4 estão mergeadas.

Implemente apenas a Fase 5. Nenhuma regra de domínio, consulta ou rota muda:
só templates e o CSS.

- Dar ao painel uma identidade visual própria (paleta, tipografia, layout),
  sem framework de CSS e sem build. Fontes embutidas no binário, sem CDN.
- Risco legível de relance: a cor forte fica reservada para o nível de risco.
- Aviso de sync com falha continua visível em toda página, mas compacto
  (o texto do erro abre sob demanda).
- Matriz: células clicáveis inteiras, número principal em destaque, célula
  parada distinguível sem depender de cor.
- Funcionar em tela estreita (celular) sem quebrar a matriz.
- Foco de teclado visível e contraste adequado.

Ao terminar: go vet, go test ./..., screenshots antes/depois no PR e a
descrição da seção 14.
```

---

## Dica de revisão para cada PR

Antes de aprovar, peça numa **sessão nova** (sem o contexto da implementação):

```
Leia o CONTEXT.md e o diff deste PR. Não resuma o que foi feito.
Liste apenas: (1) onde o código diverge do CONTEXT.md, (2) casos de erro
não tratados, (3) testes que faltam para as regras de domínio da seção 8.
```

Melhor ainda se for outro modelo (Codex ou Gemini). E sempre rode você mesmo `go test ./...` e abra o painel no navegador antes de mergear.
