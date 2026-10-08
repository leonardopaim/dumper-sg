# Migração: DumperSG com core Go e web local

Status: aplicação Go/web implementada e executável Windows gerado. Testes automatizados, cliente HTTP independente e interface em navegador aprovados. Integração real de backup/restore e cancelamento aprovada com Docker Engine no Ubuntu WSL2, sem Docker Desktop. O legado Python/PySide6 permanece em app/.

## Arquitetura implementada

Um processo Go expõe uma API HTTP em loopback. O core concentra perfis, validações, backup, restore e histórico; adaptadores cuidam de SQLite, Docker e HTTP. Frontends independentes e clientes de testes usam o mesmo contrato.

```text
Frontend web A / Frontend web B / cliente de testes
                        |
              API HTTP/JSON local
                        |
              Casos de uso em Go
                 /            \
             SQLite        Executor Docker
                              |
                       MyDumper/MyLoader
```

A biblioteca padrão oferece [HTTP](https://pkg.go.dev/net/http), [execução de processos](https://pkg.go.dev/os/exec) e [assets embarcados](https://pkg.go.dev/embed). É possível distribuir um frontend estático junto do executável e trocar ou testar outros frontends separadamente. Docker e suas imagens continuam sendo dependências externas.

Estrutura implementada:

```text
cmd/dumpersg/             inicialização e configuração
internal/core/           regras e casos de uso sem dependência de HTTP
internal/jobs/           estado, execução, cancelamento e eventos
internal/adapters/http/  rotas e DTOs públicos
internal/adapters/sqlite/ persistência e migrações
internal/adapters/docker/ argumentos, execução e logs
web/                     primeiro frontend, sem regras exclusivas de domínio
```

Um módulo Go usa a biblioteca padrão para HTTP e modernc.org/sqlite para persistência sem CGO. React, TypeScript e Vite atendem o frontend modular; suas dependências ficam separadas das regras do core.

## Comportamentos observados que precisam ser preservados

| Área atual | Destino proposto | Critério de paridade |
| --- | --- | --- |
| `app/models/` | tipos do core | Perfis e parâmetros de backup/restore continuam representáveis. |
| `app/services/mydumper_service.py` | adaptador Docker | Compressão, SSL, threads, regex, diretório e modo sem bloqueio mantêm o comportamento. |
| `app/services/myloader_service.py` | adaptador Docker | Montagem do backup, banco destino e opção de sobrescrita são explícitos. |
| `app/services/process_service.py` | executor e jobs | Inicialização, stdout/stderr, exit code, falha e cancelamento ficam observáveis. |
| `app/storage/` | adaptador SQLite | Perfis, settings e históricos são preservados. |
| `app/ui/restore_tab.py` e `app/utils/validators.py` | validações do core | Restore só local e em banco diferente do padrão do perfil, inclusive por chamadas diretas à API. |

O restore atual aceita `localhost`, `127.0.0.1`, `::1` e `host.docker.internal`. A comparação com o banco padrão ignora diferenças de maiúsculas/minúsculas. Essas regras não podem depender da interface web. A criação de banco destino também precisa de política explícita no core.

O backup sem bloqueio atualmente usa `--sync-thread-lock-mode=NO_LOCK`, `--trx-tables`, `--skip-ddl-locks` e `--no-backup-locks`. Preservar a escolha operacional e informar as limitações de consistência já descritas no README; confirmar suporte das flags na imagem escolhida.

## Contrato HTTP

| Operação | Rota | Resposta/comportamento |
| --- | --- | --- |
| Saúde do serviço | `GET /api/v1/health` | Estado do serviço; disponibilidade Docker pode ser diagnóstico separado. |
| Listar/criar perfis | `GET/POST /api/v1/profiles` | DTOs públicos sem senha nas respostas. |
| Atualizar/excluir perfil | `PATCH/DELETE /api/v1/profiles/{id}` | IDs estáveis e erros de validação documentados. |
| Testar conexão | `POST /api/v1/profiles/{id}/test` | Resultado ou job, com timeout e erro sem segredos. |
| Iniciar backup/restore | `POST /api/v1/backups`, `POST /api/v1/restores` | `202` com ID do job após validação no core. |
| Consultar job | `GET /api/v1/jobs/{id}` | Estado, progresso estimado e resultado quando disponível. |
| Cancelar job | `POST /api/v1/jobs/{id}/cancel` | Idempotente; pedido de cancelamento não equivale a término confirmado. |
| Logs/eventos | `GET /api/v1/jobs/{id}/events` | Primeiro polling com cursor; SSE se necessário. |
| Histórico/settings | `GET /api/v1/history`, `GET/PATCH /api/v1/settings` | Compatibilidade com a persistência atual. |

DTOs, estados, cursores, limites, semântica de edição de senha e erros estão em [api-contract.md](api-contract.md). Os históricos legados são importados e remapeados no banco web separado.

## Pontos técnicos que condicionam a implementação

- Jobs sobrevivem ao fechamento de uma tela. Seu contexto pertence ao backend, não ao request de criação. Concorrência e conflitos precisam ser controlados no backend para múltiplas abas/clientes.
- O cancelamento de processos Go precisa de tratamento específico para Docker: encerrar o cliente não demonstra o término do container. Rastrear e encerrar somente o container do job e verificar o resultado. A [documentação de os/exec](https://pkg.go.dev/os/exec#CommandContext) descreve o cancelamento do processo cliente.
- As regex atuais incluem lookahead negativo. A sintaxe de [regexp Go](https://pkg.go.dev/regexp/syntax) não oferece lookaround; verificar a compatibilidade com Python e MyDumper antes de trocar o validador.
- O navegador não devolve um caminho arbitrário do Windows como o seletor de pastas PySide6. Definir entrada validada ou catálogo local de diretórios para backups e restores.
- Servir em `127.0.0.1` com Host/Origin restritos e proteção das operações mutáveis contra páginas externas. Frontends de desenvolvimento têm origins configuradas; senhas não aparecem em respostas e logs.
- Dados hoje ficam em `app/data/dumper_sg.sqlite3`. Usar cópia/migração versionada e política explícita de diretórios; não fazer os dois programas escreverem juntos sem validar compatibilidade.
- Reconciliar jobs interrompidos no reinício, sem repetir restores automaticamente. Limitar retenção de logs e fila para evitar crescimento indefinido.

## Etapas do plano executado

1. Extrair regras e builders de comandos para Go com executor falso. Demonstrar validações e compatibilidade de argumentos, especialmente restrições de restore.
2. Criar API local para saúde e perfis com SQLite e testes HTTP. Confirmar migração em uma cópia do banco e acesso por um cliente independente.
3. Entregar backup completo com job, histórico, logs e cancelamento. Validar Docker em ambiente isolado.
4. Entregar restore com todas as restrições no core e testes negativos pela API. Usar apenas banco local isolado nos testes reais.
5. Criar o primeiro frontend e validar um segundo cliente contra o mesmo contrato. Depois, avaliar assets embarcados e empacotamento Windows.

Para trabalhar em paralelo, o agente principal define e mantém os contratos. Delegar core/adaptadores e frontend em diretórios distintos quando suas dependências estiverem disponíveis. Manter alterações pequenas diretamente no agente principal para evitar duplicação de contexto.

Checks esperados: testes de regras, builds, testes HTTP/SQLite e concorrência dos jobs. Integrações reais com Docker/MySQL precisam de destinos isolados. Preservar CRLF e encoding dos arquivos existentes durante a migração.

## Decisões desta implementação

- Backend Go 1.26, HTTP pela biblioteca padrão e SQLite com modernc.org/sqlite (sem exigir compilador C para o build normal).
- Frontend React + TypeScript + Vite: componentes por responsabilidade, cliente HTTP único, polling de jobs/eventos e navegação independente do core.
- Assets gerados embarcados no executável Go; Vite com proxy no desenvolvimento.
- Banco web separado em %APPDATA%\DumperSG\web por padrão. Importação explícita e somente leitura do banco legado, sem substituir perfis por nome já existentes.
- Diretórios de dados configuráveis, fila sem execução paralela de operações de banco, token de sessão para mutações e Host/Origin restritos.
- Imagens Docker configuráveis para preservar o ambiente legado sem embutir dependências externas.

O contrato definitivo desta entrega está em [api-contract.md](api-contract.md). A criação do banco destino usa POST /api/v1/databases, a sessão usa GET /api/v1/session e a importação usa POST /api/v1/import/legacy.

## Aceite e transição operacional

1. Passar testes de regras de domínio e builders, inclusive destino remoto/default bloqueado e credenciais ocultadas.
2. Passar testes de persistência em SQLite temporário, reabertura, importação repetida e reconciliação de operações interrompidas.
3. Passar testes HTTP de token, Host/Origin, CRUD, jobs fora do contexto do request e erros JSON.
4. Compilar/testar frontend e gerar executável com assets, conferir navegação e estado vazio no navegador.
5. Rodar um cliente HTTP independente contra o executável usando dados temporários, sem acessar o banco real do usuário.
6. Validar backup e restore reais em MySQL descartável somente quando Docker estiver disponível. Registrar explicitamente quando essa integração não puder ser executada.
7. Importar dados reais apenas pela ação do usuário em Configurações. Manter o desktop disponível enquanto compara o comportamento necessário no ambiente de uso.

## Resultados da validação — 07/10/2026

- `go test ./...`: aprovado nos pacotes de regras, jobs, Docker, HTTP e SQLite. Testes de Docker usam CLI simulada e verificam cancelamento, falhas e confirmação de término.
- `go vet -buildvcs=false ./...` e build Windows com frontend embarcado: aprovados.
- Frontend: 12 testes aprovados; build aprovado; instalação auditada sem vulnerabilidades reportadas.
- `scripts/smoke.ps1`: executável real aprovado com dados temporários, CRUD, senha oculta, token, bloqueio de destino remoto e SPA embarcada.
- Edge headless: criação de perfil via tela/API, navegação entre as seis telas, formulários de backup/restore, alias Docker Desktop e layouts desktop/mobile aprovados; sem erros JavaScript ou overflow horizontal. Nenhuma operação de banco real foi iniciada.
- Importação validada com bancos sintéticos e repetição idempotente; dados reais não foram importados.
- Código Python preservado; novos arquivos e skills locais mantêm CRLF e UTF-8 sem BOM.

Integração real executada após identificar o ambiente: Docker Engine 28.2.2 na distribuição Ubuntu WSL2. O detector Go `-race` não foi executado: o ambiente usa CGO desabilitado e não oferece compilador C. O detector de concorrência não é substituído pelos testes simulados.

Se um término de container não puder ser confirmado, o job mantém `cleanup_required` persistido e novas operações ficam bloqueadas. A interface oferece verificação de término; o bloqueio só é liberado após confirmação da ausência dos containers reservados.

## Adaptação ao Docker Engine no WSL

- O transporte Windows aceita `auto`, `native` ou `wsl`; `-wsl-distro` permite fixar a distribuição. Nesta máquina foi confirmado Ubuntu WSL2 com Docker 28.2.2.
- O runtime selecionado fica fixo para criação, execução, cancelamento e verificação. A identidade do daemon é registrada no job e protege a recuperação após reinício ou mudança de distribuição.
- Bind mounts de caminhos Windows são convertidos com `wslpath` da mesma distribuição e verificados antes da criação. Nenhum shell é usado para montar comandos com credenciais/caminhos.
- MySQL/MyDumper/MyLoader usam TCP explícito para que `localhost` não tente um socket Unix dentro do container de cliente. A rede host pertence ao Ubuntu WSL, conforme as condições documentadas no README.
- `scripts/integration-wsl.ps1`: aprovado em MySQL 8.4.3 descartável; teste de conexão, criação de banco isolado, backup comprimido, arquivo metadata gravado no Windows e restauração com duas linhas conferidas.
- `TestWSLCancellationIntegration` com opt-in: aprovado em Alpine descartável; container chegou ao estado running, foi cancelado e sua ausência foi confirmada no mesmo daemon.
- Containers temporários foram removidos; os containers e bancos já existentes do usuário não foram alterados.

## Correções de compatibilidade — 08/10/2026

MyLoader 1.0.3-1 não aceita a opção antiga --overwrite-tables. A aplicação agora envia --drop-table=DROP somente quando overwrite_tables=true, mantendo a confirmação de sobrescrita e as restrições de destino. A classificação de logs usa a severidade estruturada do MyDumper/MyLoader em vez de considerar todo stderr como erro.

Validação: go test ./..., go vet -buildvcs=false ./... e build aprovados. Integração real no WSL com MySQL descartável aprovada para destino vazio, falha esperada em tabelas existentes sem sobrescrita e substituição explícita de tabelas com dados alterados. Dados da origem e uma tabela alheia no destino foram preservados; níveis dos logs também foram conferidos. Nenhuma restauração foi repetida nos bancos reais do usuário.

## Interface compacta — 08/10/2026

Monitor global movido para o topo, com status, duração, cancelamento, caminho e barra de progresso estimado sempre disponíveis. Logs começam ocultos e o botão Mostrar/Ocultar logs guarda uma preferência booleana no navegador; atualização de status/eventos continua com o painel fechado. Backup e restauração distribuem seus campos em duas colunas no desktop; cartões, títulos e espaços foram reduzidos, mantendo a navegação responsiva.

Medição em 1366×768, com uma operação de backup simulada: a altura do conteúdo caiu de 1513 pixels na interface anterior com logs abertos para 820 pixels na interface compacta com logs fechados. O monitor passou de y=1132 para y=80, e o botão principal ficou totalmente visível, terminando em y=741. Barras de backup/restore, persistência da preferência, cancelamento e falhas sem logs foram conferidos no Edge headless; nenhum banco real foi utilizado nessa validação visual.

Validação final desta interface: 17 testes frontend aprovados em 6 arquivos, build Vite e executável Go aprovados. QA no navegador confirmou barras de backup/restauração, logs ocultos por padrão, preferência mantida após recarregar, cancelamento sem abrir logs, erro visível e caminhos longos sem overflow em desktop/mobile. CRUD real de perfis e navegação nas seis telas também passaram com dados temporários; nenhuma operação de banco real foi executada para validar layout.
