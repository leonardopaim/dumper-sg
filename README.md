# DumperSG

Aplicação web local para criar e restaurar backups MySQL, com perfis de conexão, histórico e logs opcionais. Usa Go, React e Docker no WSL.

## Executar

Com o executável já compilado:

```powershell
.\scripts\start.ps1 -DockerRuntime wsl -WslDistro Ubuntu -MySQLImage mysql:8.4.3
```

Abra **http://127.0.0.1:8787**. Use **Ctrl+C** para encerrar.

É necessário ter Docker Engine disponível no Ubuntu/WSL e as imagens MyDumper e MySQL. Docker Desktop não é necessário. Perfis, backups e logs ficam em `%APPDATA%\DumperSG\web`.

## Compilar ou atualizar

Requer Go 1.26+, Node.js 24 e npm:

```powershell
.\scripts\build.ps1
```

O resultado é `dist\dumpersg.exe`, com a interface incluída. Go e Node.js são necessários apenas para compilar.

Se as imagens ainda não estiverem disponíveis:

```powershell
wsl.exe --distribution Ubuntu --exec docker pull mydumper/mydumper:latest
wsl.exe --distribution Ubuntu --exec docker pull mysql:8.4.3
```

## Iniciar automaticamente

Execute na sua conta do Windows:

```powershell
.\scripts\autostart.ps1 -Action Install -StartNow
```

A aplicação inicia em segundo plano ao entrar nessa conta, usando o mesmo diretório de dados. Para consultar ou remover a inicialização:

```powershell
.\scripts\autostart.ps1 -Action Status
.\scripts\autostart.ps1 -Action Remove
```

O [guia de inicialização automática e Nginx](docs/inicializacao-automatica.md) explica o proxy local opcional em **http://127.0.0.1:8788**.

## Mais informações

- [Operação, desenvolvimento, importação e testes](docs/operacao.md)
- [Contrato da API](docs/api-contract.md)
- [Plano de migração](docs/plano-go-web-local.md)
- [Versão desktop anterior](app/README.md)
