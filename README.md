# DumperSG

Aplicação web local para criar e restaurar backups MySQL, com perfis de conexão, histórico e logs opcionais. Usa Go, React e Docker (Desktop ou Engine no WSL).

## Executar

Com o executável já compilado:

```powershell
.\scripts\start.ps1
```

Abra **http://127.0.0.1:8787**. Use **Ctrl+C** para encerrar.

O Docker é detectado automaticamente: abra o **Docker Desktop com containers Linux** ou deixe o **Docker Engine no WSL** disponível. Perfis, backups e logs ficam em `%APPDATA%\DumperSG\web`. Go e Node.js são necessários apenas para compilar.

## Compilar ou atualizar

Requer Go 1.26+, Node.js 24 e npm:

```powershell
.\scripts\build.ps1
```

O resultado é `dist\dumpersg.exe`, com a interface incluída.

Se as imagens ainda não estiverem disponíveis, no Docker Desktop:

```powershell
docker pull mydumper/mydumper:latest
docker pull mysql:8.4.3
```

No WSL, use `wsl.exe --distribution Ubuntu --exec` antes de cada comando `docker`. O [guia de operação](docs/operacao.md) explica como escolher explicitamente o ambiente e acessar bancos locais.

## Iniciar automaticamente

Execute na sua conta do Windows:

```powershell
.\scripts\autostart.ps1 -Action Install -StartNow
```

A aplicação inicia em segundo plano ao entrar nessa conta, com detecção automática de Docker e o mesmo diretório de dados. Para consultar ou remover a inicialização:

```powershell
.\scripts\autostart.ps1 -Action Status
.\scripts\autostart.ps1 -Action Remove
```

O [guia de inicialização automática e Nginx](docs/inicializacao-automatica.md) explica o proxy local opcional em **http://127.0.0.1:8788**.

## Reiniciar

Use **Reiniciar** na barra superior ou execute:

```powershell
.\scripts\restart.ps1
```

Se o backend estiver travado, use `.\scripts\restart.ps1 -Force` para reiniciar a cópia da inicialização automática. O reinício normal é bloqueado durante operações ativas; o forçado pode deixar uma operação interrompida pendente de limpeza.

## Mais informações

- [Operação, desenvolvimento, importação e testes](docs/operacao.md)
- [Contrato da API](docs/api-contract.md)
- [Plano de migração](docs/plano-go-web-local.md)
- [Versão desktop anterior](app/README.md)
