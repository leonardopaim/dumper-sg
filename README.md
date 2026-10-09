# DumperSG

Aplicação web local para criar e restaurar backups MySQL, com perfis de conexão, histórico e logs opcionais. Usa Go, React e Docker (Desktop ou Engine no WSL).

## Instalar e usar

Use o instalador `DumperSG-Setup-1.0.2-windows-x64.exe` em `dist\releases`. Depois abra **DumperSG** pelo menu Iniciar ou pelo atalho da área de trabalho. O navegador abre automaticamente em **http://127.0.0.1:8787**.

Requer Docker Desktop com containers Linux ou Docker Engine no WSL. As imagens são baixadas no primeiro uso. Quem usa a aplicação não precisa de Go, Node.js, Nginx nem código-fonte.

Para iniciar automaticamente, marque **Iniciar ao entrar na minha conta do Windows** no instalador. A instalação cuida dessa opção; não instale a antiga tarefa do projeto nem execute os scripts de inicialização da cópia avulsa.

No Windows, o ícone do DumperSG fica junto ao relógio, possivelmente na seta de ícones ocultos. Pelo botão direito, use **Abrir DumperSG** ou **Encerrar DumperSG**. O encerramento é bloqueado enquanto houver operação ativa. O atalho **Encerrar DumperSG** no menu Iniciar também está disponível. Fechar o navegador mantém a aplicação em execução.

Perfis, configurações, histórico, backups e logs ficam em `%APPDATA%\DumperSG\web` e são preservados ao atualizar ou desinstalar.

## Gerar um novo instalador

Somente quem compila precisa de **Go 1.26+**, **Node.js 24 com npm** e **Inno Setup 6**. No PowerShell:

```powershell
cd C:\Leo\DumperSG
.\scripts\build-installer.ps1 -Version 1.0.2
```

O script instala as dependências web, executa os testes, compila a interface, o core e o launcher e gera `dist\releases\DumperSG-Setup-1.0.2-windows-x64.exe`, acompanhado do SHA256. Não é necessário executar `build.ps1` antes.

Se o PowerShell bloquear o script, execute:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-installer.ps1 -Version 1.0.2
```

Nas próximas versões, altere o número informado em `-Version`. Compartilhe apenas o Setup gerado; os arquivos intermediários em `dist\package-<versão>` são usados pelo build.

## Atualizar a instalação

1. Conclua ou cancele a operação em andamento e aguarde o término.
2. Escolha **Encerrar DumperSG** pela bandeja ou pelo menu Iniciar.
3. Execute o novo Setup e mantenha a pasta da instalação existente.
4. Abra **DumperSG** pelo atalho instalado. Se a página antiga continuar aberta, atualize-a com **Ctrl+F5**.

O botão **Reiniciar** na interface reinicia a aplicação instalada; a atualização do executável é feita pelo novo Setup.

## Mais informações

- [Distribuição e validação do instalador](docs/distribuicao.md)
- [Operação, desenvolvimento, importação e testes](docs/operacao.md)
- [Contrato da API](docs/api-contract.md)
- [Versão desktop anterior](app/README.md)
