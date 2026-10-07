# DumperSG — Go e web local

Aplicação local para backups/restores MySQL com **core Go, SQLite e frontend React + TypeScript + Vite**. O frontend tem componentes e cliente HTTP reutilizáveis; clientes alternativos usam a mesma API. A versão desktop Python foi preservada em `app/`.

## Recursos

- Perfis de conexão com teste de conexão, SSL e threads padrão.
- Backup MyDumper com compressão, regex de exclusão e modo sem bloqueio por padrão.
- Restore MyLoader somente em MySQL local e banco isolado, com opção explícita de sobrescrita.
- Criação do banco destino com as mesmas restrições do restore.
- Jobs independentes da tela, logs, progresso estimado, cancelamento e histórico persistido.
- Diretório de backup configurável, catálogo de backups e importação do SQLite legado.

## Executar no Windows

Requisitos de build: Go 1.26+, Node.js 24 e npm. Requisitos de operação nesta máquina: Docker Engine instalado diretamente na distribuição Ubuntu do WSL2, usuário Linux com permissão para usar Docker e imagens MyDumper/MyLoader e MySQL disponíveis. Docker Desktop não é necessário. O executável Windows chama `wsl.exe --distribution Ubuntu --exec docker` e converte as origens dos volumes com `wslpath` da mesma distribuição.

```powershell
wsl.exe --distribution Ubuntu --exec docker pull mydumper/mydumper:latest
wsl.exe --distribution Ubuntu --exec docker pull mysql:8.4
.\scripts\build.ps1
.\scripts\start.ps1 -DockerRuntime wsl -WslDistro Ubuntu
```

Abra **http://127.0.0.1:8787**. O executável em `dist/dumpersg.exe` inclui os arquivos do frontend; Node.js e Go não são necessários para rodar esse executável já compilado. Docker continua necessário para as operações de banco. Ctrl+C encerra o serviço e cancela o job ativo, aguardando a confirmação de limpeza do container por até 90 segundos. Baixe as imagens Docker previamente para evitar timeout na primeira operação.

Dados padrão: `%APPDATA%\DumperSG\web` com `dumper_sg.sqlite3`, `backups` e `logs`. Para escolher outro diretório:

```powershell
.\scripts\start.ps1 -DataDir C:\DumperSG\dados
```

## Docker diretamente no WSL

O modo padrão `auto` tenta o daemon do cliente nativo e, se não responder no Windows, seleciona WSL. `-DockerRuntime wsl -WslDistro Ubuntu` fixa explicitamente o ambiente desta máquina; `native` atende instalações que já possuem cliente/daemon acessível pelo sistema atual. A seleção permanece fixa durante toda a execução, cancelamento e limpeza. Jobs registram a identidade do daemon para impedir que uma verificação em outro ambiente libere uma operação pendente.

Caminhos como `C:\DumperSG\dados` continuam sendo informados na interface Windows. O adaptador converte somente a origem da montagem para o caminho Linux (por exemplo `/mnt/c/DumperSG/dados`) e verifica sua existência na distribuição antes de criar o container. Não é necessário alterar o PATH do Windows nem expor o socket Docker por TCP.

Com a rede host padrão, `localhost`/`127.0.0.1` nos perfis apontam para o host Linux do daemon WSL. MySQL instalado no WSL ou publicado ali pode usar esses endereços e a porta correspondente. Se MySQL roda no Windows, o acesso por loopback depende do modo mirrored do WSL; em NAT, o endereço do host Windows é diferente. `host.docker.internal` não é criado automaticamente pelo Docker Engine no WSL. As restrições de restore local permanecem no core.

O backend aceita `-mysql-image mysql:8.4.3` para aproveitar essa imagem já instalada, ou outra tag explícita. Para o script de inicialização, use `-MySQLImage mysql:8.4.3`.

## Desenvolvimento com frontends independentes

Em dois terminais:

```powershell
.\scripts\dev.ps1 -Target backend
.\scripts\dev.ps1 -Target frontend
```

O Vite atende em http://127.0.0.1:5173 e encaminha `/api` ao Go. Para outro frontend, configure uma origem exata no backend com `-origins` e obtenha o token em `GET /api/v1/session` antes de enviar `X-DumperSG-Token` nas mutações. O [contrato da API](docs/api-contract.md) documenta os DTOs e rotas. O processo Go recusa bind fora de loopback.

## Migração dos dados antigos

Em Configurações, informe o caminho absoluto do SQLite legado (normalmente `C:\Leo\DumperSG\app\data\dumper_sg.sqlite3` ou próximo do executável Python) e execute a importação. A origem é aberta somente para leitura. Perfis são mesclados por nome sem substituir os já cadastrados; histórico recebe IDs remapeados e importação repetida da mesma origem é idempotente. Revise o diretório de backups importado nas configurações.

Não utilize o mesmo arquivo SQLite como banco simultâneo das duas versões. O novo banco fica separado por padrão. A aplicação não importa nem executa backups/restores automaticamente.

## Limites operacionais

Um job de banco por vez evita conflitos locais. Se a limpeza de um container não puder ser confirmada, novas operações ficam bloqueadas até a verificação de término; o bloqueio também sobrevive ao reinício do serviço. Progresso intermediário é estimado; sucesso depende do término da ferramenta. O modo sem bloqueio mantém as limitações do legado: DDL concorrente ou tabelas não transacionais podem comprometer a consistência do backup.

As senhas continuam armazenadas localmente no SQLite, como no desktop; elas não são retornadas pela API nem gravadas nos eventos de jobs. Proteja o diretório de dados com as permissões do Windows. Logs têm retenção limitada em memória e arquivos por job. As versões de imagem podem ser configuradas por `-docker-image` e `-mysql-image`; confirme o suporte às opções no ambiente de destino.

## Validação

```powershell
go test ./...
go vet -buildvcs=false ./...
.\scripts\smoke.ps1
cd web
npm test -- --run
npm run build
```

Os testes Go usam SQLite temporário e executor falso; não acessam bancos reais. O smoke inicia o executável com dados isolados e valida também os assets embarcados, sem executar backup/restore. Validação real de Docker/MySQL requer um banco local isolado. Consulte o [plano de migração](docs/plano-go-web-local.md).

Teste real e isolado no WSL, usando imagens já instaladas (não faz pull nem acessa bancos existentes):

```powershell
.\scripts\integration-wsl.ps1 -WslDistro Ubuntu -MySQLImage mysql:8.4.3
$env:DUMPERSG_TEST_WSL_INTEGRATION = 'Ubuntu'
go test ./internal/adapters/docker -run '^TestWSLCancellationIntegration$' -count=1 -v
Remove-Item Env:DUMPERSG_TEST_WSL_INTEGRATION
```

A integração cria um MySQL descartável, testa conexão/criação de banco/backup/restore, compara os dados restaurados e remove seu container/volumes próprios. Os artefatos ficam em `data/integration-wsl-*` (ignorados pelo Git). O teste Go adicional verifica cancelamento com Alpine descartável. Ambos foram aprovados no Ubuntu WSL desta máquina.
