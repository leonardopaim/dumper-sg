import { useOperationRequest } from "../components/OperationFeedback";
import { useEffect, useRef, useState } from "react";
import { Archive, ArrowRight } from "lucide-react";
import { api, errorMessage } from "../api";
import {
  useTableSelection,
  tableSelectionKey,
} from "../hooks/useTableSelection";
import { PresetPicker } from "../components/PresetPicker";
import { TableSelector } from "../components/TableSelector";
import { backupDraftKey, useOperationDraft } from "../hooks/useOperationDraft";
import {
  Alert,
  Button,
  Empty,
  Field,
  PageHeader,
  Toggle,
} from "../components/ui";
import type {
  BackupInput,
  BackupSource,
  Job,
  Profile,
  Settings,
} from "../types";
export function Backup({
  profiles,
  settings,
  active,
  onJob,
  onProfiles,
  initialSource,
  onSourceApplied,
  onBrowseDatabases,
}: {
  profiles: Profile[];
  settings: Settings;
  active: boolean;
  onJob: (job: Job) => void;
  onProfiles: () => void;
  initialSource?: BackupSource;
  onSourceApplied?: () => void;
  onBrowseDatabases?: () => void;
}) {
  const [form, setForm] = useOperationDraft<BackupInput>(
    backupDraftKey,
    () => ({
      profile_id: profiles[0]?.id || 0,
      database: profiles[0]?.database || "",
      destination_dir: "",
      threads: 0,
      compress: true,
      ssl: profiles[0]?.ssl || false,
      non_locking: true,
      ignore_regex: "",
    }),
  );
  const runOperation = useOperationRequest();
  const databaseInput = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const tableSelection = useTableSelection(
    { profile_id: form.profile_id, database: form.database, ssl: form.ssl },
    onJob,
  );
  const emptySelection = tableSelection.selection?.length === 0;
  const profile = profiles.find((item) => item.id === form.profile_id);
  const production =
    profile?.host.trim().toLowerCase().replace(/\.$/, "") ===
    "db.sommusgestor.com";
  const effectiveThreads = form.threads || profile?.threads || 8;
  const unsafeThreads = production && effectiveThreads > 2;
  const [advancedOpen, setAdvancedOpen] = useState(false);
  useEffect(() => {
    if (unsafeThreads) setAdvancedOpen(true);
  }, [unsafeThreads]);
  const threadsAlert =
    "Backup em db.sommusgestor.com permite no máximo 2 threads por segurança. Cada thread aumenta o número de conexões no banco de dados de produção.";
  const selectProfile = (id: number) => {
    const selected = profiles.find((item) => item.id === id);
    setForm((data) => ({
      ...data,
      profile_id: id,
      database: selected?.database || "",
      ssl: selected?.ssl || false,
    }));
  };
  useEffect(() => {
    const sourceProfile =
      initialSource &&
      profiles.find((item) => item.id === initialSource.profile_id);
    if (initialSource && sourceProfile) {
      setForm((data) => ({
        ...data,
        ...initialSource,
        ssl: sourceProfile.ssl,
      }));
      onSourceApplied?.();
      return;
    }
    if (
      profiles.length &&
      !profiles.some((item) => item.id === form.profile_id)
    )
      selectProfile(profiles[0]?.id || 0);
    else {
      const current = profiles.find((item) => item.id === form.profile_id);
      if (!form.database.trim() && current?.database)
        setForm((data) => ({ ...data, database: current.database }));
    }
  }, [profiles, form.profile_id, profile?.database, initialSource]);
  const update = <K extends keyof BackupInput>(key: K, value: BackupInput[K]) =>
    setForm((data) => ({ ...data, [key]: value }));
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    if (unsafeThreads) {
      setError(threadsAlert);
      return;
    }
    if (emptySelection || tableSelection.loading || active) {
      setError(
        emptySelection
          ? "Selecione ao menos uma tabela ou use todas as tabelas."
          : "Aguarde a operação em andamento.",
      );
      return;
    }
    setBusy(true);
    try {
      const data = { ...form, tables: tableSelection.selection };
      if (new TextEncoder().encode(JSON.stringify(data)).length > 65536)
        throw new Error(
          "A seleção excede o limite de tamanho da requisição. Reduza a lista ou use todas as tabelas no modo automático.",
        );
      onJob(await runOperation("backup", () => api.backup(data)));
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <PageHeader
        eyebrow="PROTEÇÃO DE DADOS"
        title="Criar backup"
        description="Escolha a origem e onde salvar o backup."
      />
      {profiles.length === 0 ? (
        <Empty title="Nenhum perfil de conexão">
          <Button onClick={onProfiles}>Configurar um perfil</Button>
        </Empty>
      ) : (
        <form onSubmit={submit} className="operation-layout">
          <div className="card form-card operation-card">
            <section className="operation-section">
              <div className="card-heading">
                <div>
                  <h2>Origem e destino</h2>
                </div>
                {onBrowseDatabases && (
                  <Button
                    type="button"
                    variant="ghost"
                    className="browse-databases"
                    onClick={onBrowseDatabases}
                  >
                    Buscar banco
                  </Button>
                )}
              </div>
              <div className="form-grid">
                <Field label="Perfil de conexão" className="full">
                  <select
                    value={form.profile_id}
                    onChange={(e) => selectProfile(Number(e.target.value))}
                  >
                    {profiles.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name} · {item.host}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field
                  label="Banco de origem"
                  hint={
                    !form.database.trim()
                      ? "Informe o nome do banco aqui para habilitar a consulta de tabelas."
                      : undefined
                  }
                >
                  <input
                    ref={databaseInput}
                    aria-label="Banco de origem"
                    required
                    value={form.database}
                    onChange={(e) => update("database", e.target.value)}
                    placeholder="nome_do_banco"
                  />
                </Field>
                <Field
                  label="Diretório de destino"
                  hint={
                    settings.default_backup_dir
                      ? `Vazio usa o padrão: ${settings.default_backup_dir}`
                      : "Informe um caminho absoluto ou defina um diretório padrão em Configurações."
                  }
                >
                  <input
                    value={form.destination_dir}
                    onChange={(e) => update("destination_dir", e.target.value)}
                    placeholder={
                      settings.default_backup_dir ||
                      "Caminho absoluto no computador"
                    }
                  />
                </Field>
              </div>
            </section>
            {unsafeThreads && (
              <Alert warning>
                {threadsAlert} Threads efetivas: {effectiveThreads}.
              </Alert>
            )}
            <details
              className="advanced-options"
              open={advancedOpen}
              onToggle={(event) => setAdvancedOpen(event.currentTarget.open)}
            >
              <summary>
                Opções avançadas{" "}
                <small>
                  {effectiveThreads} threads ·{" "}
                  {form.compress ? "Comprimido" : "Sem compressão"}
                </small>
              </summary>
              <div className="form-grid">
                <Field
                  label="Threads"
                  hint={`0 usa o perfil (${profile?.threads || 8} threads).`}
                >
                  <input
                    type="number"
                    min={0}
                    max={128}
                    required
                    value={form.threads}
                    onChange={(e) => update("threads", Number(e.target.value))}
                  />
                </Field>
                <Field
                  label="Excluir tabelas por expressão"
                  hint="Opcional. Expressão regular compatível com mydumper."
                >
                  <input
                    value={form.ignore_regex}
                    onChange={(e) => update("ignore_regex", e.target.value)}
                    placeholder="Ex.: .*\.logs_.*"
                  />
                </Field>
              </div>
              <div className="toggle-list">
                <Toggle
                  label="Comprimir arquivos"
                  hint="Reduz o espaço utilizado pelo backup."
                  checked={form.compress}
                  onChange={(value) => update("compress", value)}
                />
                <Toggle
                  label="Usar SSL"
                  hint="Utiliza conexão criptografada com o servidor."
                  checked={form.ssl}
                  onChange={(value) => update("ssl", value)}
                />
                <Toggle
                  label="Não bloquear tabelas"
                  hint="Evita locks; considere a consistência dos dados durante gravações."
                  checked={form.non_locking}
                  onChange={(value) => update("non_locking", value)}
                />
              </div>
            </details>

            <TableSelector
              controller={tableSelection}
              active={active}
              canQuery={!!profile && !!form.database.trim()}
              scope={tableSelectionKey(form.profile_id, form.database)}
              ignoreRegex={form.ignore_regex}
              missingReason={
                !profile
                  ? "Selecione um perfil de conexão acima."
                  : "Informe o banco de origem acima para consultar as tabelas."
              }
              onInvalidQuery={() => databaseInput.current?.focus()}
              extra={
                <PresetPicker
                  profiles={profiles}
                  onApply={(source, preset) => {
                    tableSelection.applyTo(
                      source.id,
                      preset.database,
                      preset.tables,
                    );
                    setForm((data) => ({
                      ...data,
                      profile_id: source.id,
                      database: preset.database,
                      ssl: source.ssl,
                    }));
                  }}
                />
              }
            />
            {emptySelection && (
              <Alert>
                Nenhuma tabela selecionada. Ajuste a seleção de tabelas antes de
                iniciar.
              </Alert>
            )}
            {error && <Alert>{error}</Alert>}
            <div className="form-footer">
              {active && <span>Aguarde a operação em andamento.</span>}
              <Button
                type="submit"
                disabled={
                  active ||
                  tableSelection.loading ||
                  emptySelection ||
                  unsafeThreads
                }
                busy={busy}
              >
                <Archive size={17} />
                Iniciar backup
                <ArrowRight size={16} />
              </Button>
            </div>
          </div>
        </form>
      )}
    </>
  );
}
