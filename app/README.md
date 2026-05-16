# DumperSG

Aplicacao desktop Windows para backups e restores MySQL com MyDumper/MyLoader em Docker.

## Requisitos

- Windows com Docker Desktop usando WSL2
- Python 3.12+
- Docker acessivel no `PATH`
- MySQL acessivel a partir do container Docker

## Instalar dependencias

```powershell
cd C:\Leo\DumperSG\app
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
```

## Executar

```powershell
cd C:\Leo\DumperSG\app
python main.py
```

## Build Windows

```powershell
cd C:\Leo\DumperSG\app
pyinstaller --onefile --windowed main.py
```

O executavel sera gerado em `dist\main.exe`.

## Docker

A aplicacao executa comandos Docker com `QProcess`, sem bloquear a interface. O padrao usa:

```powershell
docker run --rm --network host -v "C:\Backups:/backup" mydumper/mydumper:latest mydumper ...
```

Em Docker Desktop, se `--network host` nao acessar o MySQL do Windows em algum ambiente, use o host `host.docker.internal` no perfil.

## Operacao sem bloqueio do banco

Por padrao, backups sao executados com o modo "Nao bloquear banco" ativo. Esse modo adiciona:

```powershell
--sync-thread-lock-mode=NO_LOCK --trx-tables --skip-ddl-locks --no-backup-locks
```

Isso evita solicitar `FLUSH TABLES WITH READ LOCK`, `LOCK TABLE` e locks de DDL/backup quando possivel. Use esse modo preferencialmente em databases com tabelas InnoDB. Se existirem tabelas nao transacionais ou DDL acontecendo durante o backup, o backup pode ficar inconsistente, mas a prioridade operacional e nao bloquear usuarios.

Restores devem ser feitos em um database isolado. A aplicacao bloqueia restore quando o destino e igual ao database padrao do perfil, pois restaurar ou sobrescrever tabelas em um banco usado por usuarios pode causar locks e impacto direto.

Por seguranca, restores so sao permitidos em MySQL local. Hosts aceitos: `localhost`, `127.0.0.1`, `::1` e `host.docker.internal`. Perfis apontando para servidores externos podem ser usados para backup, mas nao para restore.

## Persistencia e logs

- SQLite: `data\dumper_sg.sqlite3`
- Logs: `logs\app.log`, `logs\backup.log`, `logs\restore.log`
- Backups locais padrao: `backups\`

## Observacoes de seguranca

As senhas dos perfis sao salvas localmente no SQLite para simplificar a operacao desktop. Em evolucoes futuras, recomenda-se proteger esse campo com Windows Credential Manager ou DPAPI.

