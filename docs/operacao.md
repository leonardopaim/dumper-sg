# Guia de operação do DumperSG

Aplicação local para backups/restores MySQL com **core Go, SQLite e frontend React + TypeScript + Vite**. O frontend tem componentes e cliente HTTP reutilizáveis; clientes alternativos usam a mesma API. A versão desktop Python foi preservada em `app/`.

## Recursos

- Perfis de conexão com teste de conexão, SSL e threads padrão.
- Backup MyDumper com seleção de tabelas por tamanho, compressão, regex de exclusão e modo sem bloqueio por padrão.
- Restore MyLoader somente em MySQL local e banco isolado, com confirmação por resumo dos dados e opção explícita de sobrescrita (`--drop-table=DROP`, compatível com MyLoader 1.0.3).
- Criação do banco destino com as mesmas restrições do restore.
- Jobs independentes da tela, monitor abaixo do conteúdo principal, barra de progresso estimado, cancelamento e histórico persistido.
- Layout compacto com campos em colunas no desktop. Painel de processos e logs oculto por padrão, com controle geral Mostrar painel/Ocultar painel sempre acessível na barra superior; a preferência é salva no navegador. A exibição dos eventos dentro do painel tem controle separado.
- Diretório de backup configurável, catálogo de backups e importação do SQLite legado.

## Executar no Windows

Requisitos de build: Go 1.26+, Node.js 24 e npm. Para executar, use Docker Desktop com containers Linux ou Docker Engine em uma distribuição WSL2, com permissão para usar Docker e imagens MyDumper/MyLoader e MySQL disponíveis. O modo padrão detecta o Docker acessível pelo Windows e, se indisponível, tenta a distribuição WSL padrão. Nesta máquina o Engine está no Ubuntu/WSL.

```powershell
wsl.exe --distribution Ubuntu --exec docker pull mydumper/mydumper:latest
wsl.exe --distribution Ubuntu --exec docker pull mysql:8.4.3
.\scripts\build.ps1
.\scripts\start.ps1
```

Abra **http://127.0.0.1:8787**. O executável em `dist/dumpersg.exe` inclui os arquivos do frontend; Node.js e Go não são necessários para rodar esse executável já compilado. Docker continua necessário para as operações de banco. Ctrl+C encerra o serviço e cancela o job ativo, aguardando a confirmação de limpeza do container por até 90 segundos. Baixe as imagens Docker previamente para evitar timeout na primeira operação.

No Windows, o ícone do DumperSG aparece na bandeja junto ao relógio, inclusive ao iniciar em segundo plano. Clique para abrir a interface, ou use o botão direito para **Abrir DumperSG** e **Encerrar DumperSG**. O encerramento pela bandeja é bloqueado enquanto houver operação ativa ou cancelamento em andamento; o Windows mostra um aviso e mantém a aplicação aberta. Fechar o navegador mantém o backend em execução. O ícone pode ficar na seta de ícones ocultos. Para execução sem bandeja, passe `-no-tray` diretamente ao executável.

Dados padrão: `%APPDATA%\DumperSG\web` com `dumper_sg.sqlite3`, `backups` e `logs`. Para escolher outro diretório:

```powershell
.\scripts\start.ps1 -DataDir C:\DumperSG\dados
```

## Escolher Docker Desktop ou WSL

O modo padrão `auto` tenta o daemon do cliente nativo e, se não responder no Windows, tenta a distribuição WSL padrão. Se nenhum daemon estiver disponível na abertura, a interface continua acessível e a descoberta é tentada novamente ao verificar o Docker ou executar uma operação. Assim, o Docker Desktop pode ser aberto depois do DumperSG. Após encontrar um daemon, o transporte permanece fixo até reiniciar o backend. Jobs registram sua identidade para impedir que uma verificação em outro ambiente libere uma operação pendente.

Para escolher explicitamente:

```powershell
.\scripts\start.ps1 -DockerRuntime native
.\scripts\start.ps1 -DockerRuntime wsl -WslDistro Ubuntu
```

No Docker Desktop, mantenha o aplicativo aberto e o modo **Linux containers**. O contexto ativo do cliente Docker determina o daemon nativo usado. O DumperSG reconhece Desktop pelo `docker info`, preserva os caminhos Windows dos volumes e usa a rede padrão dos containers. Perfis com `localhost`, `127.0.0.1` ou `::1` são traduzidos para `host.docker.internal` somente no comando executado; o perfil salvo permanece igual. Endereços remotos são preservados. Isso dispensa habilitar host networking no Desktop. Se houver restrição de compartilhamento de arquivos no Desktop, libere o diretório escolhido para os backups.

Os diagnósticos informam **Docker Desktop** ou **Docker Engine** e o transporte escolhido. Se ambos estiverem instalados, `auto` prefere o daemon acessível pelo Windows. Imagens são locais a cada daemon; ao trocar de ambiente, baixe-as no daemon selecionado.

Referências: [rede no Docker Desktop](https://docs.docker.com/desktop/features/networking/networking-how-tos/) e [host networking e requisito de habilitação no Desktop](https://docs.docker.com/engine/network/drivers/host/).

No transporte WSL, caminhos como `C:\DumperSG\dados` continuam sendo informados na interface Windows. O adaptador converte somente a origem da montagem para o caminho Linux (por exemplo `/mnt/c/DumperSG/dados`) e verifica sua existência na distribuição antes de criar o container. Não é necessário alterar o PATH do Windows nem expor o socket Docker por TCP.

No Docker Engine no WSL, com a rede host padrão, `localhost`/`127.0.0.1` nos perfis apontam para o host Linux do daemon WSL. MySQL instalado no WSL ou publicado ali pode usar esses endereços e a porta correspondente. Se MySQL roda no Windows, o acesso por loopback depende do modo mirrored do WSL; em NAT, o endereço do host Windows é diferente. `host.docker.internal` não é criado automaticamente pelo Docker Engine no WSL. As restrições de restore local permanecem no core.

O backend aceita `-mysql-image mysql:8.4.3` para aproveitar essa imagem já instalada, ou outra tag explícita. Para o script de inicialização, use `-MySQLImage mysql:8.4.3`.

## Desenvolvimento com frontends independentes

Em dois terminais:

```powershell
.\scripts\dev.ps1 -Target backend
.\scripts\dev.ps1 -Target frontend
```

O Vite atende em http://127.0.0.1:5173 e encaminha `/api` ao Go. Para outro frontend, configure uma origem exata no backend com `-origins` e obtenha o token em `GET /api/v1/session` antes de enviar `X-DumperSG-Token` nas mutações. O [contrato da API](api-contract.md) documenta os DTOs e rotas. O processo Go recusa bind fora de loopback.

## Migração dos dados antigos

Em Configurações, informe o caminho absoluto do SQLite legado (normalmente `C:\Leo\DumperSG\app\data\dumper_sg.sqlite3` ou próximo do executável Python) e execute a importação. A origem é aberta somente para leitura. Perfis são mesclados por nome sem substituir os já cadastrados; histórico recebe IDs remapeados e importação repetida da mesma origem é idempotente. Revise o diretório de backups importado nas configurações.

Não utilize o mesmo arquivo SQLite como banco simultâneo das duas versões. O novo banco fica separado por padrão. A aplicação não importa nem executa backups/restores automaticamente.

## Limites operacionais

Backups no host `db.sommusgestor.com` aceitam no máximo 2 threads efetivas, inclusive quando herdadas do perfil. Valores maiores são bloqueados na interface e no core: cada thread aumenta o número de conexões no banco de dados de produção. As opções avançadas do backup mostram as threads efetivas e permitem corrigir o valor.

Um job de banco por vez evita conflitos locais. Se a limpeza de um container não puder ser confirmada, novas operações ficam bloqueadas até a verificação de término; o bloqueio também sobrevive ao reinício do serviço. Progresso intermediário é estimado; sucesso depende do término da ferramenta. O modo sem bloqueio mantém as limitações do legado: DDL concorrente ou tabelas não transacionais podem comprometer a consistência do backup.

As senhas continuam armazenadas localmente no SQLite, como no desktop; elas não são retornadas pela API nem gravadas nos eventos de jobs. Proteja o diretório de dados com as permissões do Windows. Logs têm retenção limitada em memória e arquivos por job. As versões de imagem podem ser configuradas por `-docker-image` e `-mysql-image`; confirme o suporte às opções no ambiente de destino.

## Interface e acompanhamento

Backup e restauração usam formulários centrais com opções avançadas recolhidas. A origem da restauração alterna entre backup salvo e pasta manual. Perfis são apresentados em lista; ações secundárias ficam em Mais ações. Configurações concentra aparência, armazenamento e manutenção (Docker, reinício e importação do legado).

Ao disparar uma ação, o acompanhamento abre já na preparação. Falhas anteriores à criação de um job também aparecem no modal. Durante a execução, ele mostra etapa atual, progresso estimado, tempo e últimos acontecimentos. Os logs completos podem ser expandidos. Minimizar ou fechar a janela não cancela o processamento; use Ver operação no topo para reabri-la. Ao finalizar, a janela mostra sucesso, alertas, resultado parcial, erro ou cancelamento e permanece aberta até ser fechada. Histórico abre o mesmo modal. Resultado parcial significa que a consulta de bancos não conseguiu obter parte das informações opcionais de grupos/empresas; seleção intencional de tabelas continua sendo uma execução normal.

## Selecionar tabelas e confirmar restauração

Em Criar backup, abra o módulo de tabelas e carregue a lista do perfil/banco de origem. Os objetos aparecem do maior para o menor, com busca e seleção em lote. O modo padrão inclui todas as tabelas sem precisar consultar a lista. Ao escolher **Seleção personalizada** antes da primeira consulta, o catálogo carrega com todas marcadas. A caixa no cabeçalho **Tabela** marca ou desmarca o catálogo inteiro, inclusive tabelas ocultas pela busca, e indica quando a seleção é parcial. Seleções salvas ou alteradas não são substituídas ao atualizar o catálogo; no modo personalizado, selecione pelo menos uma. Preencha o banco de origem no formulário ou no perfil antes de consultar; o botão informa e foca o campo quando ele está vazio. A escolha é lembrada separadamente por perfil e banco. A consulta usa o Docker configurado e não pode executar junto de outro job.

O tamanho é uma estimativa de dados e índices informada pelo MySQL, não o tamanho final dos arquivos comprimidos. Views também aparecem na lista. Tabelas relacionadas não são incluídas automaticamente e a regex de exclusão continua valendo sobre os objetos selecionados. Ao atualizar o catálogo, revise objetos adicionados/removidos.

Em Perfis de banco, use **Seleções de tabelas** para consultar a origem e salvar conjuntos nomeados. Eles ficam no SQLite junto do perfil e podem ser aplicados no backup ou na restauração. Na restauração, escolha a pasta e abra a seleção para carregar os objetos dos arquivos, ordenados pelo tamanho exportado. O conjunto identifica banco e nomes de origem e pode vir de outro perfil; isso não altera o perfil de destino. Revise nomes ausentes e dependências antes de executar. O modo personalizado exige ao menos uma tabela. Restore parcial não inclui rotinas/eventos globais.

Ao restaurar, confira o resumo com perfil, servidor, banco destino, pasta, threads e sobrescrita; confirme ou cancele pelo botão. Não é necessário redigitar o banco. Sobrescrever tabelas continua desmarcado ao voltar ao formulário e o destino continua restrito a um banco local diferente do padrão do perfil.

Referências técnicas: [metadados e tamanhos no MySQL](https://dev.mysql.com/doc/refman/8.4/en/information-schema-tables-table.html) e [filtros de tabelas do MyDumper](https://mydumper.github.io/mydumper/docs/html/regex.html).

## Validação

```powershell
go test ./...
go vet -buildvcs=false ./...
.\scripts\smoke.ps1
cd web
npm test -- --run
npm run build
```

Os testes Go usam SQLite temporário e executor falso; não acessam bancos reais. O smoke inicia o executável com dados isolados e valida também os assets embarcados, sem executar backup/restore. Validação real de Docker/MySQL requer um banco local isolado. Consulte o [plano de migração](plano-go-web-local.md).

Teste real e isolado no WSL, usando imagens já instaladas (não faz pull nem acessa bancos existentes):

```powershell
.\scripts\integration-wsl.ps1 -WslDistro Ubuntu -MySQLImage mysql:8.4.3
$env:DUMPERSG_TEST_WSL_INTEGRATION = 'Ubuntu'
go test ./internal/adapters/docker -run '^TestWSLCancellationIntegration$' -count=1 -v
Remove-Item Env:DUMPERSG_TEST_WSL_INTEGRATION
```

A integração cria um MySQL descartável, testa conexão/criação de banco/backup/restore, rejeita tabelas existentes sem sobrescrita, valida a sobrescrita explícita e os níveis dos logs, compara os dados restaurados e remove seu container/volumes próprios. Também confere catálogo por tamanho, views, banco inexistente e backups seletivos com nomes especiais e regex cumulativa, seleções persistidas no perfil, catálogo de arquivos e restauração parcial que preserva tabelas desmarcadas. Os artefatos ficam em `data/integration-wsl-*` (ignorados pelo Git). O teste Go adicional verifica cancelamento com Alpine descartável. Ambos foram aprovados no Ubuntu WSL desta máquina.

## Gestão dos arquivos de backup

Em **Meus backups**, consulte tamanho e quantidade de arquivos, busque por nome/caminho e ordene por data ou tamanho. **Ver detalhes** lista tabelas e views de backups finalizados; **Abrir pasta** acessa os arquivos no computador onde o core está executando. **Restaurar** preenche a origem no formulário sem iniciar a operação.

O catálogo inclui o diretório padrão e pastas personalizadas registradas nas operações de backup, incluindo backups incompletos. **Excluir** pede confirmação e remove permanentemente a pasta inteira; o histórico é preservado. A exclusão fica bloqueada durante operações ou pendências de término de containers. Links e junctions não são seguidos para calcular tamanho ou excluir arquivos.

Os menus de seleção usam opções com bordas arredondadas nos navegadores compatíveis com selects personalizáveis (por exemplo, Chrome/Edge atuais). Navegadores sem suporte mantêm o controle nativo. Referência: [selects personalizáveis no navegador](https://developer.mozilla.org/en-US/docs/Learn_web_development/Extensions/Forms/Customizable_select).

## Campos lembrados na interface

Em **Configurações → Aparência**, escolha o tema **Claro** ou **Escuro**. A mudança é aplicada imediatamente a toda a interface e lembrada neste navegador. O tema claro é o padrão; com o armazenamento bloqueado, ainda é possível alternar o tema durante a sessão.

Os perfis cadastrados e o diretório padrão são persistidos no SQLite da aplicação. Os formulários de backup e restauração lembram separadamente o último perfil e os valores preenchidos no navegador, incluindo pasta, banco e threads. A seleção é recuperada por ID mesmo se a lista de perfis mudar de ordem; se o perfil for removido, um perfil disponível é selecionado e o destino de restauração é limpo. A opção de sobrescrever tabelas não é memorizada.

Esses campos são salvos automaticamente, sem credenciais, no armazenamento do navegador para a origem usada. localhost, 127.0.0.1 e as portas 8787/8788 têm armazenamentos independentes. Use o mesmo endereço ao voltar; limpar os dados do site também remove essas preferências. Se o navegador bloquear o armazenamento, os formulários continuam funcionando com seus valores padrão.

## Bancos disponíveis e nomes dos grupos

Em **Bancos disponíveis**, escolha o perfil de conexão e clique em **Carregar bancos**. Pesquise pelo nome do grupo, razão social, nome fantasia, ID do grupo ou database; a busca ignora maiúsculas, acentos e espaços extras. O modo **Automática** encontra nomes por palavras e trata IDs e databases completos como valores exatos: `sommusgestor_1` não inclui `sommusgestor_10`. Escolha **Igual (exata)** para comparar o nome completo ou **Contém (parcial)** para procurar trechos, inclusive prefixos de databases. **Preparar backup** abre o formulário com o perfil, database e SSL correspondentes, preservando as demais opções preenchidas. A exportação começa somente ao clicar em **Iniciar backup**. O formulário também oferece um acesso direto à lista.

A lista mostra os databases visíveis para o usuário do perfil, exceto os quatro schemas de sistema do MySQL. Quando há bancos `sommusgestor_<id>`, consulta os nomes em `sommusgestor.grupo_empresa` usando grupos ativos da instância 1. Consulta também `sommusgestor.empresa` para associar razão social e nome fantasia pelo `grupo_empresa_id`, excluindo registros marcados como excluídos e restringindo os grupos à instância 1. Empresas não excluídas são incluídas mesmo quando `ativo = 0`, pois continuam pertencendo ao banco do grupo. Selecionar pelo nome de uma empresa prepara o backup do banco inteiro do grupo; não restringe os dados a essa empresa. A leitura não usa os dados de conexão de `instancia_banco_dados`. Grupos sem database visível não são oferecidos; databases sem grupo correspondente continuam disponíveis pelo nome técnico. Se grupos ou empresas não puderem ser consultados, um aviso explica a ausência; a lista de bancos e os metadados que puderam ser obtidos continuam disponíveis.

As consultas usam o Docker configurado e o mesmo controle de execução das outras operações. Trocar de perfil ou sair da tela cancela a consulta dessa tela e descarta resultados antigos. **Atualizar bancos** refaz a consulta; o catálogo não é persistido. O limite de 10.000 bancos é explícito e não retorna uma lista truncada. As consultas opcionais de grupos e empresas têm limite de 10.000 registros cada; erros ou excesso descartam os dados dessa consulta e exibem aviso.
