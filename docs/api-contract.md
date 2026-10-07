# Contrato da API local v1

Todas as rotas estão sob `/api/v1`. JSON com nomes snake_case. Listagens retornam arrays (vazios como `[]`). Erros: `{ "error": "mensagem" }`. Validação: 400; não encontrado: 404; conflito/operação ativa: 409. POST de job: 202 e objeto Job. Datas RFC3339 UTC. IDs de perfil numéricos; job IDs string.

## Sessão e frontend

`GET /session` retorna `{ "token": "..." }`; requisições mutáveis exigem `X-DumperSG-Token`. Frontend e API usam a mesma origem no executável; Vite proxy encaminha `/api` a `http://127.0.0.1:8787`. Clientes independentes obtêm a sessão antes de mutações. Host restrito a loopback; Origin, quando enviado, deve ser autorizado. Token nunca em URL. Não há endpoints que aceitem comandos arbitrários.

## Perfis

`GET /profiles` e `GET /profiles/{id}` retornam Profile público: id, name, host, port, user, database, ssl, threads, has_password. Senha não é retornada.

`POST /profiles`: name, host, port, user, password, database, ssl, threads. Port padrão 3306; threads padrão 8. `PATCH /profiles/{id}` aceita os mesmos campos: ausente mantém, password vazio limpa a senha. `DELETE /profiles/{id}` retorna 204. `POST /profiles/{id}/test` cria job connection_test sem body.

## Operações

- `POST /backups`: profile_id, database, destination_dir, threads, compress, ssl, non_locking (padrão true), ignore_regex. Diretório vazio usa configuração default_backup_dir; threads 0 usa perfil.
- `POST /restores`: profile_id, backup_dir, target_database, threads, overwrite_tables. Restore local e banco destino diferente do padrão do perfil, validados no core.
- `POST /databases`: profile_id, database. Mesmas restrições de destino local/isolado do restore.
- `GET /jobs` e `GET /history?limit=100`: array Job, recentes primeiro. Job: id, kind (backup, restore, connection_test, create_database), status (running, cancel_requested, succeeded, failed, cancelled), profile_id, profile_name, database, path, started_at, finished_at?, progress (0-100, estimado), message, exit_code?.
- `GET /jobs/{id}` retorna Job; `POST /jobs/{id}/cancel` retorna Job. Cancelar job finalizado é idempotente.
- `GET /jobs/{id}/events?after=0` retorna array Event: sequence, time, level, message. Polling incremental; retenção limitada. Ao selecionar job histórico, a UI informa caso os eventos não estejam mais em memória.

## Configuração e diagnóstico

- `GET /health`: `{ "status": "ok", "version": "1.0.0" }`.
- `GET /diagnostics`: `{ "available": bool, "version": string, "message": string }` para Docker.
- `GET /settings`: mapa string/string contendo default_backup_dir. `PATCH /settings` recebe o mesmo mapa parcial.
- `GET /backups`: array `{ "path": string, "name": string, "modified_at": string }` de diretórios com metadata de backup, sob default_backup_dir.
- `POST /import/legacy`: `{ "path": "caminho absoluto do SQLite legado" }`; importa em transação sem alterar origem. Retorna `{ "profiles": número, "jobs": número }`. IDs importados são remapeados; conflitos por nome mantêm perfil já existente. Frontend oferece configuração e ação explícita.

Não guardar credenciais no localStorage. O backend persiste senhas localmente, como o legado; seu DTO público e logs as ocultam. Um job por vez para evitar conflitos de Docker e destinos; conexões independem da duração das telas.

## Término não confirmado

Jobs com cleanup_required=true permanecem failed e bloqueiam novas operações até confirmar a remoção do container próprio. A proteção é persistida e vale após reinício. Remova o container dumpersg-<job-id> no daemon Docker da operação depois de verificar seu estado; ao tentar novamente, o core verifica os containers pendentes antes de liberar execução. POST /jobs/{id}/cancel pode repetir a verificação sem transformar uma falha de limpeza em sucesso.

O campo opcional `docker_identity` do job identifica o transporte/distribuição e daemon usados. A confirmação de limpeza exige a mesma identidade; mudar de daemon ou distribuição não libera pendências. Jobs antigos sem identidade precisam ser verificados pelo transporte nativo original, antes de migrar para WSL.
