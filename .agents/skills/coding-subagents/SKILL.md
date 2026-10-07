---
name: coding-subagents
description: Coordenar subagentes para implementar código no DumperSG quando houver frentes independentes, escopo de escrita separável e autorização de delegação. Aplicar a implementação paralela e revisão focal; resolver alterações simples diretamente.
---

# Subagentes para implementação

Delegue quando o trabalho independente justificar o custo de instruir, acompanhar e integrar. Paralelismo pode reduzir latência sem reduzir tokens; não considere isso economia automática.

No DumperSG, os contratos compartilhados são docs/api-contract.md e internal/core/types.go. Frentes típicas separáveis: core/jobs/Docker, SQLite, frontend web e integração HTTP/CLI. Distribua apenas as frentes exigidas pela alteração atual.

## Decidir a divisão

- Confirme autorização da sessão e disponibilidade das ferramentas. Sem autorização aplicável, execute diretamente ou obtenha a autorização necessária. Esta skill não supera restrições superiores.
- Resolva localmente mudanças pequenas, tarefas sequenciais e tarefas cujo principal custo seja compreender o mesmo arquivo.
- Estabeleça primeiro contratos compartilhados: DTOs, rotas, erros, exemplos e fronteira entre legado e novo. Paralelize API e frontend depois desses contratos.
- O agente principal mantém integração e contratos compartilhados. Cada subagente recebe arquivos/diretórios exclusivos para escrita; revisões podem ser somente leitura.
- Comece com o menor número útil de agentes e respeite os slots disponíveis. Não crie um agente por arquivo nem delegação recursiva sem necessidade concreta.

## Contexto e contrato de entrega

Use ferramentas de colaboração para subtarefas. Não crie chats visíveis ao usuário para simular subagentes; criar um chat é operação distinta que exige pedido do usuário.

Prefira contexto mínimo e explícito; com spawn_agent, use fork_turns="none" quando o briefing for autossuficiente:

~~~text
Objetivo: comportamento observável a implementar.
Escrita permitida: arquivos/diretórios exclusivos.
Contexto: diretório absoluto, instruções aplicáveis, símbolos e referências essenciais.
Contrato: interfaces/DTOs, erros, invariantes e encoding/quebras de linha.
Aceite: checks ou exemplos que provam o comportamento.
Limites: escopo, dependências, ações proibidas e bloqueios conhecidos.
Retorno: arquivos alterados, resultado, checks e pendências; não colar arquivos inteiros.
~~~

- Inclua restrições relevantes do usuário e AGENTS.md; o fork sem histórico não as transmite automaticamente.
- Preserve modelo e esforço herdados por padrão. Não escolha modelos supostamente mais baratos sem autorização e dados aplicáveis.
- Em revisão independente, forneça pedido e artefatos sem induzir a conclusão.
- Informe quando o checkout for compartilhado. Não peça cherry-pick de alterações já presentes no mesmo diretório.
- Worktrees são opção quando houver necessidade e autorização, não requisito universal. Não faça commits, push ou alterações globais apenas para coordenar.

## Acompanhar e integrar

- Execute trabalho complementar enquanto os agentes trabalham. Não repita a implementação ou as buscas delegadas.
- Prefira eventos e esperas compatíveis com os updates da sessão, sem polling frequente. Resolva dependências em caso de bloqueio.
- Comunique mudanças de interface antes de os agentes dependentes continuarem. Se surgir sobreposição de escrita, interrompa e redistribua a propriedade.
- Inspecione o diff, resolva incompatibilidades e execute checks de integração apropriados. Conclusão individual não demonstra que o conjunto funciona.
- Reutilize checks que ainda cubram o estado final; repita os afetados pela integração.
- Entregue o resultado integrado, evidências e limitações; coordenação interna não é funcionalidade entregue.
