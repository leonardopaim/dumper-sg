# Inicialização automática e Nginx

## Como funciona nesta máquina

O DumperSG inicia **ao entrar na sua conta do Windows**, por uma tarefa no Agendador de Tarefas. A tarefa roda com permissões normais, sem armazenar senha e sem abrir janela. Mantém o processo Go em execução, permite apenas uma instância da tarefa e tenta reiniciá-lo até três vezes, com intervalo de um minuto, se ele falhar. Não há limite de duração nem encerramento ao usar bateria.

O Nginx instalado em `C:\nginx` utiliza WinSW 2.12 em `service\nginx-service.exe`, com serviço `SommusNginx` executado como `LocalSystem`. O wrapper inicia o próprio Nginx; o Nginx não inicia o backend Go. Reutilizar essa conta para o DumperSG perderia acesso ao WSL e ao diretório de dados da sua conta. Por isso o backend usa a tarefa de login, e o Nginx existente pode atendê-lo como proxy.

A aplicação fica disponível em `http://127.0.0.1:8787`. O Docker no Ubuntu/WSL deve estar disponível para executar operações de banco; sua ausência não impede abrir a interface. Ao sair da conta ou desligar o Windows, o processo termina. Isso não é execução antes do login.

## Instalar e consultar

Com `dist\dumpersg.exe` compilado, execute na conta proprietária do WSL:

```powershell
.\scripts\autostart.ps1 -Action Install -StartNow
.\scripts\autostart.ps1 -Action Status
```

Sem `-StartNow`, a tarefa começa no próximo login. Não inicie outra cópia manual na mesma porta. `Install` não reconfigura uma tarefa em execução. Para outro diretório de dados ou imagem, informe as opções na instalação:

```powershell
.\scripts\autostart.ps1 -Action Install -DataDir C:\DumperSG\dados -WslDistro Ubuntu -MySQLImage mysql:8.4.3
```

A tarefa tem nome `DumperSG-<SID da conta>`. A configuração fica em `data\autostart-<SID>.json` no projeto, sem credenciais de banco. Mantenha o projeto e o executável nesse caminho; reinstale a tarefa se mover a pasta. Logs da inicialização ficam em `<diretório de dados>\startup\stdout.log` e `stderr.log`, com uma cópia `.previous` da execução anterior. Os logs dos backups/restores continuam em `logs`.

Para atualizar o executável, conclua os jobs, saia da aplicação e então compile. Não substitua um executável em execução. Se precisar encerrar a cópia automática, use o Agendador de Tarefas para finalizar a tarefa e confira se `dumpersg.exe` encerrou; evite esse procedimento durante backup/restore. Ao reiniciar, o core verifica containers de operações interrompidas antes de liberar novas operações.

## Proxy opcional no Nginx existente

O arquivo `deploy\nginx\dumpersg.conf` adiciona um servidor exclusivo, ouvindo somente **127.0.0.1:8788**, encaminhado ao backend em **127.0.0.1:8787**. O instalador copia apenas esse arquivo para `C:\nginx\conf\sites-enabled\dumpersg.conf`, testa a configuração inteira com `nginx -t` e solicita reload. Os arquivos do SommusGestor não são alterados. Se a validação falhar, o arquivo anterior é restaurado.

```powershell
.\scripts\configure-nginx.ps1
```

Abra **http://127.0.0.1:8788** ou **http://localhost:8788**. Nesta máquina, execute o comando em um **PowerShell como administrador**: o serviço Nginx roda como `LocalSystem` e exige elevação para receber o sinal de reload. Se o reload for negado, o instalador restaura a configuração anterior.

O backend iniciado automaticamente já permite essas duas origens exatas. Para iniciar manualmente com o proxy:

```powershell
.\scripts\start.ps1 -DockerRuntime wsl -WslDistro Ubuntu -MySQLImage mysql:8.4.3 -Origins 'http://127.0.0.1:8788,http://localhost:8788'
```

O proxy preserva o cabeçalho Origin, rejeita outros Host e mantém as validações de sessão do backend. As portas continuam acessíveis somente na própria máquina. Para outra instalação Nginx, use `-NginxDir`; ela precisa incluir `conf/sites-enabled/*.conf` dentro de `http`. `-NoReload` apenas instala e valida, deixando a ativação para um reload posterior. `-WhatIf` mostra a ação sem modificar arquivos.

## Remover

```powershell
.\scripts\autostart.ps1 -Action Remove
.\scripts\configure-nginx.ps1 -Remove
```

Remover a tarefa impede os próximos inícios automáticos e deixa uma aplicação já em execução continuar, para preservar jobs ativos. Remover o proxy mantém o acesso direto pela porta 8787. Os dados da aplicação são preservados.

## Referências

- [Nginx no Windows e comandos de reload](https://nginx.org/en/docs/windows.html)
- [Execução do WSL sob contas Windows](https://learn.microsoft.com/en-us/windows/wsl/faq)
- [Identidade da tarefa no Agendador](https://learn.microsoft.com/en-us/powershell/module/scheduledtasks/new-scheduledtaskprincipal)

## Verificação isolada

```powershell
.\scripts\smoke.ps1
.\scripts\smoke-startup.ps1
```

O segundo teste usa o Nginx de `C:\nginx` com configuração temporária, portas livres e SQLite separado. Verifica o launcher no Windows PowerShell, caminhos com espaços, propagação de falha, favicon, proxy, CRUD de perfis fictícios e proteção de Host/Origin/token. Não instala tarefas, não altera a configuração do serviço Nginx e não executa operações em bancos MySQL. Os artefatos ficam em `data\startup qa *`.
