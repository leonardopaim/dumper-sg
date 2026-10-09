# Distribuir DumperSG no Windows

Entregue apenas **DumperSG-Setup-1.0.0-windows-x64.exe**, gerado em `dist\releases`. O instalador inclui executáveis, interface, ícone e instruções; não inclui perfis, senhas, backups ou dados da máquina de quem compilou.

## Para quem vai usar

1. Disponibilize Docker Desktop com containers Linux ou Docker Engine na distribuição padrão do WSL.
2. Execute o instalador e abra **DumperSG** pelo menu Iniciar ou pelo atalho da área de trabalho.
3. Cadastre seus próprios perfis e use a aplicação no navegador.

A instalação é por usuário em `%LOCALAPPDATA%\Programs\DumperSG`, para Windows 10/11 de 64 bits. Não precisa de Go, Node.js, Nginx, PowerShell de configuração ou acesso ao código. A instalação do Docker é um pré-requisito separado e pode depender da política da máquina. Se o Docker ainda não estiver disponível, a interface abre e mostra o diagnóstico.

As imagens MyDumper/MySQL são baixadas automaticamente na primeira operação que precisar delas. O primeiro uso requer internet para acessar o registro Docker e pode levar alguns minutos; o processo informa a preparação no painel e aceita cancelamento. Imagens existentes são reutilizadas.

O instalador oferece atalhos e uma opção para iniciar em segundo plano ao entrar na conta atual. Fechar o navegador mantém o core funcionando. Use **Encerrar DumperSG** no menu Iniciar para sair, ou **Reiniciar** na interface para reiniciar. O encerramento é bloqueado durante backup/restore e outras operações ativas.

## Atualização e desinstalação

Encerre a aplicação pelo atalho e execute o novo instalador. Perfis, histórico, configurações, backups e logs permanecem em `%APPDATA%\DumperSG\web`; a desinstalação também preserva esses dados. O instalador não fecha processos automaticamente nem interrompe operações. Uma cópia instalada em execução bloqueia instalação e desinstalação.

Logs do launcher ficam em `%APPDATA%\DumperSG\web\installed-launcher`. Se já existe outra cópia do DumperSG na porta 8787, abrir o atalho reutiliza a interface existente; encerrar essa outra cópia continua sendo responsabilidade do launcher dela.

## Gerar e validar um pacote

No computador de desenvolvimento, com Go, Node.js e [Inno Setup 6](https://jrsoftware.org/isinfo.php):

```powershell
.\scripts\build-installer.ps1 -Version 1.0.0
```

O script testa e compila sem sobrescrever `dist\dumpersg.exe` em uso. O compilador do Inno Setup pode ser indicado por `-Compiler`. `-SkipWebBuild` reutiliza o frontend já compilado; `-SkipTests` permite usar verificações feitas previamente. A preparação intermediária fica em `dist\package-<versão>`, e o instalador recebe um arquivo `.sha256` para conferir integridade.

Teste de instalação completo em uma conta Windows sem um pacote DumperSG já instalado:

```powershell
.\scripts\smoke-installer.ps1
```

O teste usa pasta/porta/SQLite isolados, sem atalhos ou inicialização automática, e valida instalação, abertura repetida, reinício, encerramento, reinstalação e desinstalação preservando dados. Não acessa bancos MySQL existentes. O download de imagens e seu cancelamento têm testes com CLI simulado; a detecção de Docker Desktop e WSL também é coberta pelo projeto.

O pacote gerado localmente não é assinado digitalmente. Para distribuição institucional com editor verificado, a assinatura deve usar um certificado de publicação autorizado pelo responsável pelo projeto.
