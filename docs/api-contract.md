# Contrato da API local v1

Todas as rotas estão sob `/api/v1`. JSON com nomes snake_case. Listagens retornam arrays (vazios como `[]`). Erros: `{ "error": "mensagem" }`. Validação: 400; não encontrado: 404; conflito/operação ativa: 409. POST de job: 202 e objeto Job. Datas RFC3339 UTC. IDs de perfil numéricos; job IDs string.

## Sessão e frontend

`GET /session` retorna `{ "token": "..." }`; requisições mutáveis exigem `X-DumperSG-Token`. Frontend e API usam a mesma origem no executável; Vite proxy encaminha `/api` a `http://127.0.0.1:8787`. Clientes independentes obtêm a sessão antes de mutações. Host restrito a loopback; Origin, quando enviado, deve ser autorizado. Token nunca em URL. Não há endpoints que aceitem comandos arbitrários.

## Perfis

`GET /profiles` e `GET /profiles/{id}` retornam Profile público: id, name, host, port, user, database, ssl, threads, has_password, table_presets. Senha não é retornada.

`POST /profiles`: name, host, port, user, password, database, ssl, threads, table_presets. Port padrão 3306; threads padrão 8. `PATCH /profiles/{id}` aceita os mesmos campos: ausente mantém, password vazio limpa a senha. `DELETE /profiles/{id}` retorna 204. `POST /profiles/{id}/test` cria job connection_test sem body.

`table_presets` guarda seleções nomeadas no perfil: `{ "name": "Essenciais", "database": "origem", "tables": ["clientes", "pedidos"] }`. Máximo de 20 seleções por perfil e 10.000 nomes exatos por seleção. Nome de seleção obrigatório, até 64 caracteres, sem duplicatas; banco e lista não podem ser vazios. `PATCH` só desse campo preserva conexão e senha; ausente/null mantém, `[]` remove todas. O banco padrão do perfil pode continuar vazio. Seleções podem ser reutilizadas no backup e no restore, inclusive com outro perfil de destino.

## Operações

- `POST /backups`: profile_id, database, destination_dir, threads, compress, ssl, non_locking (padrão true), ignore_regex, tables. Diretório vazio usa configuração default_backup_dir; threads 0 usa perfil. tables ausente/null inclui todas; array de nomes exatos inclui somente os objetos selecionados. Array vazio é inválido. A regex de exclusão continua cumulativa. Nomes são tratados literalmente, inclusive caracteres especiais; seleção que exceda 16 KiB de expressão é rejeitada.
- `POST /tables`: profile_id, database, ssl. Cria job table_list para consultar metadados do banco de origem usando o mesmo Docker e controle de execução das demais operações; não exporta dados. database vazio usa o padrão do perfil. Um banco inexistente/inacessível gera falha.
- `GET /jobs/{id}/tables`: após sucesso do job table_list, retorna array `{ "name": string, "size_bytes": número, "rows": número, "table_type": "BASE TABLE" | "VIEW" }`, ordenado por tamanho decrescente e nome. Tamanho estimado = DATA_LENGTH + INDEX_LENGTH; rows também pode ser estimativa. Não representa o tamanho comprimido do backup. Limite de 10.000 objetos; exceder causa falha explícita. Resultado incompleto/falho não é retornado (409); resultado não retido, após reinício ou de outro tipo de job retorna 404. O catálogo é transitório; seleções do frontend são lembradas por perfil/banco no navegador.
- `POST /backups/tables`: `{ "backup_dir": "caminho" }`. Requer sessão. Lê apenas arquivos locais de um backup completo e retorna array `{ "database": string, "name": string, "size_bytes": número, "rows": número, "table_type": "BASE TABLE" | "VIEW" }`, ordenado por tamanho decrescente. O tamanho soma arquivos exportados (incluindo compressão); rows vem do metadata. Nomes reais de objetos são recuperados mesmo quando o MyDumper usa aliases nos arquivos. Backup incompleto ou catálogo não interpretável retorna 400, sem lista parcial.
- `POST /restores`: profile_id, backup_dir, target_database, threads, overwrite_tables, tables. tables ausente/null restaura tudo; array de referências `{ "database": "origem_no_backup", "name": "nome_exato" }` restaura somente os objetos selecionados. Array vazio, objetos ausentes ou colisões de nomes entre bancos de origem são rejeitados. O filtro parcial inclui os nomes reais e os aliases dos arquivos do banco de origem, rejeita aliases ambíguos que incluiriam objetos desmarcados, inclui o schema de criação do banco e omite objetos globais de pós-processamento (rotinas/eventos); dependências precisam ser selecionadas explicitamente. Restore local e banco destino diferente do padrão do perfil, validados no core. overwrite_tables=false não permite remover tabelas existentes; true usa a opção explícita --drop-table=DROP do MyLoader para recriar as tabelas restauradas.
- `POST /databases`: profile_id, database. Mesmas restrições de destino local/isolado do restore.
- `GET /jobs` e `GET /history?limit=100`: array Job, recentes primeiro. Job: id, kind (backup, restore, connection_test, create_database, table_list), status (running, cancel_requested, succeeded, failed, cancelled), profile_id, profile_name, database, path, started_at, finished_at?, progress (0-100, estimado), message, exit_code?.
- `GET /jobs/{id}` retorna Job; `POST /jobs/{id}/cancel` retorna Job. Cancelar job finalizado é idempotente.
- `GET /jobs/{id}/events?after=0` retorna array Event: sequence, time, level, message. Polling incremental; retenção limitada. Ao selecionar job histórico, a UI informa caso os eventos não estejam mais em memória.

## Configuração e diagnóstico

- `GET /health`: `{ "status": "ok", "version": "1.0.0" }`.
- `GET /application`: `{ "instance_id": string, "restart_available": bool }`. A identificação muda em cada processo, independentemente do token de sessão. O reinício está disponível quando iniciado pelos scripts supervisores.
- `POST /application/restart`: body `{}`, sessão obrigatória. Retorna 202 `{ "status": "restarting", "instance_id": "instância anterior" }`; 409 se há operação ativa ou outro reinício em andamento; 501 quando não há supervisor. A verificação de ociosidade e o bloqueio de novos jobs são atômicos. Pendências de containers são preservadas e continuam bloqueando novas operações após o reinício. O backend encerra HTTP/SQLite e o launcher abre outro processo com os mesmos argumentos. Aguarde um `instance_id` diferente e obtenha uma nova sessão antes de mutações.
- `GET /diagnostics`: `{ "available": bool, "version": string, "message": string }` para Docker.
- `GET /settings`: mapa string/string contendo default_backup_dir. `PATCH /settings` recebe o mesmo mapa parcial.
- `GET /backups`: array `{ "path": string, "name": string, "modified_at": string }` de diretórios com metadata de backup, sob default_backup_dir.
- `POST /import/legacy`: `{ "path": "caminho absoluto do SQLite legado" }`; importa em transação sem alterar origem. Retorna `{ "profiles": número, "jobs": número }`. IDs importados são remapeados; conflitos por nome mantêm perfil já existente. Frontend oferece configuração e ação explícita.

Não guardar credenciais no localStorage. O backend persiste senhas localmente, como o legado; seu DTO público e logs as ocultam. Um job por vez para evitar conflitos de Docker e destinos; conexões independem da duração das telas.

## Término não confirmado

Jobs com cleanup_required=true permanecem failed e bloqueiam novas operações até confirmar a remoção do container próprio. A proteção é persistida e vale após reinício. Remova o container dumpersg-<job-id> no daemon Docker da operação depois de verificar seu estado; ao tentar novamente, o core verifica os containers pendentes antes de liberar execução. POST /jobs/{id}/cancel pode repetir a verificação sem transformar uma falha de limpeza em sucesso.

O campo opcional `docker_identity` do job identifica o transporte/distribuição e daemon usados. A confirmação de limpeza exige a mesma identidade; mudar de daemon ou distribuição não libera pendências. Jobs antigos sem identidade precisam ser verificados pelo transporte nativo original, antes de migrar para WSL.

Os níveis de eventos seguem os prefixos estruturados do MyDumper/MyLoader: Message/INFO/DEBUG como info, WARNING como warning e ERROR/CRITICAL como error, mesmo quando escritos em stderr. Texto não reconhecido preserva o nível do fluxo; status final do job depende do exit code e da confirmação de término.

Avisos do cliente MySQL com prefixo `mysql: [Warning]` são classificados como warning. Linhas de dados do catálogo não são registradas como eventos nem gravadas nos logs; o job informa somente a conclusão e a quantidade de objetos.
