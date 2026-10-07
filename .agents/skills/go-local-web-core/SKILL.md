---
name: go-local-web-core
description: Planejar ou implementar a migração do DumperSG para core Go com API HTTP local e frontends independentes. Aplicar quando Go e execução web local fizerem parte do pedido; não iniciar migração apenas por menção exploratória.
---

# Core Go com interface web local

Distinga planejamento de implementação. Uma intenção permite avaliar arquitetura; implemente a migração quando fizer parte do pedido. Preserve comportamentos, dados e invariantes do legado.

Consulte o [plano do DumperSG](../../../docs/plano-go-web-local.md) para mapa do legado, restrições de restore e ordem de entrega. Confira o status e os resultados de validação registrados; propostas ainda não implementadas devem ser distinguidas do código já verificado.

## Arquitetura mínima

- Separe regras, casos de uso e adaptadores. O core não depende de framework HTTP, DOM, estado visual ou frontend específico.
- Comece com um processo e um módulo Go. Prefira biblioteca padrão quando atender; não introduza microserviços, broker, DI container ou ORM sem necessidade.
- Defina a API antes de repartir API e frontend: rotas, DTOs públicos, códigos HTTP, erros, estados e exemplos. Documentação e versão do contrato devem corresponder à implementação.
- Frontends usam a mesma API. Validações de domínio ficam no core para impedir contorno por outro cliente; DTOs e transporte ficam no adaptador HTTP.
- Isole persistência e execução externa em interfaces pequenas na fronteira de uso para testar casos de uso sem Docker, rede ou banco reais.
- Dados, configuração, logs e backups ficam em diretórios explícitos e estáveis, fora dos assets. Preserve o banco por migração versionada ou importação controlada, sem sobrescrevê-lo.
- Pode servir frontend estático pelo binário com embed e executar outros frontends separadamente. Não imponha uma biblioteca de interface antes do requisito.

## Execução e validação

Para API, jobs ou empacotamento, leia somente os tópicos necessários de [API e jobs locais](references/local-api-jobs.md).

- Migre uma fatia vertical verificável por vez, começando por core testável e operação simples. Preserve a aplicação anterior até demonstrar paridade.
- Confira argumentos, caminhos Windows, regex, estados, dados e dependências em vez de traduzir linha a linha.
- Execute checks Go adequados: formatação, go test dos pacotes afetados, build e go vet quando aplicável; use -race para concorrência se suportado. Respeite quebras de linha exigidas pelo projeto, inclusive na formatação.
- Use executor falso para unidade e testes HTTP em processo para contratos. Docker/MySQL reais entram em integração autorizada com destinos isolados.
- Comprove independência do frontend com outro cliente HTTP ou de testes. Uma tela funcionando não prova o contrato.
- Explicite dependências: um binário Go não elimina Docker, MyDumper/MyLoader ou um driver SQLite quando mantidos no desenho.
