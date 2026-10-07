# API e jobs locais

Leia apenas as seções relacionadas à alteração.

## HTTP local

- Bind explícito em loopback, por exemplo 127.0.0.1. Configure porta, timeouts, limite de corpo e shutdown.
- Restrinja Host e Origin ao serviço e frontends autorizados. Proteja operações mutáveis/execução contra páginas externas e CSRF; loopback e CORS isoladamente não são autenticação.
- Prefira mesma origem para o frontend embarcado. Para desenvolvimento, configure origins exatas e autenticação apropriada; não use CORS universal.
- Não devolva senhas em perfis, jobs ou logs. Redija segredos nas saídas do processo. O navegador não acessa caminhos locais como o file picker desktop: defina catálogo de diretórios, entrada validada ou integração local específica.
- Canonicalize caminhos e aplique limites do caso de uso, considerando escape por symlinks/junctions quando pertinente.

## Jobs longos

- Retorne ID e estado consultável para operações aceitas. Use transições atômicas e estados explícitos: queued, running, succeeded, failed, cancel_requested e cancelled, por exemplo.
- O contexto do job pertence ao gerenciador, não à duração do POST ou à conexão de uma tela.
- Controle concorrência, fila e conflitos no backend; um botão desabilitado não impede outra aba ou cliente.
- Publique logs/progresso com sequência e retenção limitada. Comece com polling; use SSE para eventos unidirecionais quando necessário. Suporte reconexão sem buffers ou goroutines ilimitados.
- Progresso estimado não significa sucesso. Estado final depende do executor; preserve erros de inicialização e distinção de cancelamento.
- Reconcilie jobs persistidos no reinício. Não marque sucesso nem reexecute automaticamente, especialmente restores.
- Cancelamento repetido é idempotente; confirme cancelled só após verificar término externo.

## Processos e Docker

- Use executável e argumentos separados com os/exec. Evite shell com entrada do usuário; flags e identificadores SQL ainda exigem validação.
- Use contexto, leitura concorrente de stdout/stderr, exit code e limites de leitura. Execute Wait e libere recursos.
- Matar o cliente docker run não prova término do container. Identifique o container do job, pare somente esse container e verifique término/limpeza.
- Verifique cancelamento no Windows e no Docker usado; não suponha comportamento idêntico a sinais Unix.
- Confirme versões/capacidades antes de fixar flags. Não altere modos de backup, locks ou restore sem evidência e requisito.
- Python, Go e ferramentas externas têm sintaxes de regex diferentes. Go regexp não aceita lookaround; não o use cegamente para validar expressões destinadas ao MyDumper.

## Referências oficiais sob demanda

- [HTTP](https://pkg.go.dev/net/http)
- [Processos e cancelamento](https://pkg.go.dev/os/exec)
- [Assets embarcados](https://pkg.go.dev/embed)
- [Sintaxe de regexp Go](https://pkg.go.dev/regexp/syntax)
