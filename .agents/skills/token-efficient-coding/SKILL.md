---
name: token-efficient-coding
description: Implementar, corrigir ou refatorar código no DumperSG com economia de contexto e tokens, usando exploração direcionada, alterações pequenas e validação proporcional. Aplicar em tarefas de programação; não substituir skills específicas de artefatos ou pesquisas.
---

# Implementação com contexto enxuto

Otimize o custo total até a entrega correta: tokens, retrabalho e verificações. Não prometa percentuais de economia sem medição; encurtar a resposta não compensa implementar parcialmente.

Na aplicação web, comece pela área afetada em internal/core, internal/jobs, internal/adapters, cmd/dumpersg ou web/src. O legado Python fica em app/; leia-o apenas para paridade ou manutenção explícita. Consulte README.md para execução, docs/api-contract.md para contratos e docs/plano-go-web-local.md para migração; não carregue todas as áreas por padrão.

## Exploração e execução

- Identifique o resultado esperado, as restrições e a evidência de aceite. Para mudanças pequenas, avance diretamente; registre um plano curto somente quando houver dependências relevantes.
- Use rg --files para localizar candidatos e rg -n para encontrar símbolos. Leia a implementação afetada e seus chamadores; amplie somente por dependência ou dúvida concreta.
- Não carregue árvores completas, arquivos grandes, lockfiles ou logs integrais por padrão. Consulte trechos com linhas e preserve caminhos para voltar à fonte; não trate saída truncada como resultado completo.
- Agrupe buscas e leituras independentes. Execute edições, operações dependentes e decisões adaptativas em sequência. Limite saídas mantendo a evidência necessária.
- Reutilize o que já foi verificado. Reabra fontes modificadas, inclusive por outro agente, ou quando houver indício de desatualização.
- Reutilize padrões e dependências existentes. Adicione abstrações apenas por duplicação real ou necessidade de isolamento.
- Preserve encoding e quebras de linha de cada arquivo; em projetos Sommus, preserve CRLF. Em arquivos novos, siga a convenção do diretório. Não reformate áreas não relacionadas.
- Entregue uma alteração funcional por vez. Não inclua modernizações ou limpeza alheia ao pedido.

## Validação e encerramento

- Verifique o diff, execute checks exigidos pelo projeto e testes relacionados ao comportamento alterado. Testes devem detectar regressões reais; não apenas reproduzir a implementação.
- Amplie checks quando houver impacto em integração, concorrência, persistência ou segurança. Após aprovação, repita somente por nova alteração ou evidência de problema.
- Diagnostique a causa antes de repetir um comando falho. Leia o trecho relevante do erro e não esconda falhas para reduzir saída.
- Resuma o que mudou, a validação realizada e limitações materiais. Não narre buscas rotineiras nem cole patches e logs já disponíveis.
- Para tarefas longas, preserve objetivo, decisões, arquivos alterados, checks e próximo passo em um resumo compacto; não copie a conversa.
- Use subagentes somente com autorização aplicável e ferramentas disponíveis, para tarefa independente que justifique contexto adicional. A economia vem de evitar exploração duplicada, não de multiplicar agentes.
