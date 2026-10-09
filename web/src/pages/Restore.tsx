import { useOperationRequest } from "../components/OperationFeedback";
import { useEffect, useRef, useState } from "react";
import { RotateCcw, Database, RefreshCw } from "lucide-react";
import { TableSelector } from "../components/TableSelector";
import { PresetPicker } from "../components/PresetPicker";
import {
  useRestoreTables,
  restoreSelectionKey,
} from "../hooks/useRestoreTables";
import { api, errorMessage } from "../api";
import { restoreDraftKey, useOperationDraft } from "../hooks/useOperationDraft";
import {
  Alert,
  Button,
  Empty,
  Field,
  Modal,
  PageHeader,
  Toggle,
  dateTime,
} from "../components/ui";
import type { BackupEntry, Job, Profile, RestoreInput } from "../types";
const transientFields: (keyof RestoreInput)[] = ["overwrite_tables"];
export function Restore({
  profiles,
  backups,
  reloadBackups,
  onJob,
  active,
  onProfiles,
  initialBackup,
  onBackupApplied,
}: {
  profiles: Profile[];
  backups: BackupEntry[];
  reloadBackups: () => Promise<void>;
  onJob: (job: Job) => void;
  active: boolean;
  onProfiles: () => void;
  initialBackup?: string;
  onBackupApplied?: () => void;
}) {
  const [form, setForm] = useOperationDraft<RestoreInput>(
    restoreDraftKey,
    () => ({
      profile_id: profiles[0]?.id || 0,
      backup_dir: "",
      target_database: "",
      threads: 0,
      overwrite_tables: false,
    }),
    transientFields,
  );
  const [sourceMode, setSourceMode] = useState<"catalog" | "manual">(() =>
    backups.length &&
    (!form.backup_dir || backups.some((item) => item.path === form.backup_dir))
      ? "catalog"
      : "manual",
  );
  const runOperation = useOperationRequest();
  const backupInput = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (initialBackup) {
      setSourceMode(
        backups.some((item) => item.path === initialBackup)
          ? "catalog"
          : "manual",
      );
      setForm((data) => ({
        ...data,
        backup_dir: initialBackup,
        overwrite_tables: false,
      }));
      onBackupApplied?.();
    }
  }, [initialBackup]);
  const restoreTables = useRestoreTables(form.profile_id, form.backup_dir);
  const emptySelection = restoreTables.selection?.length === 0;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [confirmation, setConfirmation] = useState(false);
  const profile = profiles.find((item) => item.id === form.profile_id);
  const isolated =
    !!form.target_database.trim() &&
    form.target_database.trim().toLowerCase() !==
      profile?.database.trim().toLowerCase();
  const local =
    !!profile &&
    ["localhost", "127.0.0.1", "::1", "host.docker.internal"].includes(
      profile.host.trim().toLowerCase(),
    );
  useEffect(() => {
    if (
      profiles.length &&
      !profiles.some((item) => item.id === form.profile_id)
    )
      setForm((data) => ({
        ...data,
        profile_id: profiles[0].id,
        target_database: "",
      }));
  }, [profiles, form.profile_id]);
  const update = <K extends keyof RestoreInput>(
    key: K,
    value: RestoreInput[K],
  ) => {
    setForm((data) => ({ ...data, [key]: value }));
    setNotice("");
  };
  const restore = async () => {
    setBusy(true);
    setError("");
    setConfirmation(false);
    try {
      const data = { ...form, tables: restoreTables.references };
      if (new TextEncoder().encode(JSON.stringify(data)).length > 65536)
        throw new Error(
          "A seleção excede o limite de tamanho da requisição. Reduza a lista ou restaure todas as tabelas.",
        );
      onJob(await runOperation("restore", () => api.restore(data)));
      setConfirmation(false);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    if (emptySelection || restoreTables.loading) {
      setError(
        emptySelection
          ? "Selecione ao menos uma tabela ou restaure todas as tabelas."
          : "Aguarde a consulta dos arquivos do backup.",
      );
      return;
    }
    if (!active && local && isolated) setConfirmation(true);
  };
  const createDatabase = async () => {
    setBusy(true);
    setError("");
    try {
      onJob(
        await runOperation("create_database", () =>
          api.createDatabase(form.profile_id, form.target_database),
        ),
      );
      setNotice("Criação do banco iniciada.");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const refresh = async () => {
    setBusy(true);
    setError("");
    try {
      await reloadBackups();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <PageHeader
        eyebrow="RECUPERAÇÃO"
        title="Restaurar backup"
        description="Escolha o backup e um banco local de destino."
        action={
          <Button
            variant="secondary"
            busy={busy}
            onClick={() => void refresh()}
          >
            <RefreshCw size={16} />
            Atualizar catálogo
          </Button>
        }
      />
      {profiles.length === 0 ? (
        <Empty title="Nenhum perfil de conexão">
          <Button onClick={onProfiles}>Configurar um perfil local</Button>
        </Empty>
      ) : (
        <form className="operation-layout" onSubmit={submit}>
          <div className="card form-card operation-card">
            <section className="operation-section">
              <div className="card-heading">
                <div>
                  <h2>Backup de origem</h2>
                </div>
              </div>
              <div
                className="source-mode"
                role="group"
                aria-label="Origem do backup"
              >
                {(["catalog", "manual"] as const).map((mode) => (
                  <label key={mode}>
                    <input
                      type="radio"
                      name="backup-source"
                      checked={sourceMode === mode}
                      onChange={() => {
                        setSourceMode(mode);
                        update("backup_dir", "");
                      }}
                    />
                    {mode === "catalog" ? "Backup salvo" : "Pasta manual"}
                  </label>
                ))}
              </div>
              {sourceMode === "catalog" ? (
                <Field
                  label="Backup do catálogo"
                  hint="Inclui destinos personalizados registrados no histórico. Gerencie arquivos em Meus backups."
                >
                  <select
                    required
                    value={
                      backups.some((item) => item.path === form.backup_dir)
                        ? form.backup_dir
                        : ""
                    }
                    onChange={(e) => update("backup_dir", e.target.value)}
                  >
                    <option value="">
                      {backups.length
                        ? "Selecionar um backup…"
                        : "Nenhum backup encontrado no catálogo"}
                    </option>
                    {backups.map((item) => (
                      <option
                        key={item.path}
                        value={item.path}
                        disabled={item.complete === false}
                      >
                        {item.name} · {dateTime(item.modified_at)}
                        {item.complete === false ? " · Incompleto" : ""}
                      </option>
                    ))}
                  </select>
                </Field>
              ) : (
                <Field
                  label="Diretório do backup"
                  hint="Caminho absoluto da pasta que contém os arquivos e metadata."
                >
                  <input
                    required
                    ref={backupInput}
                    value={form.backup_dir}
                    onChange={(e) => update("backup_dir", e.target.value)}
                    placeholder="Caminho absoluto no computador"
                  />
                </Field>
              )}
            </section>
            <section className="operation-section">
              <div className="card-heading section-heading">
                <div>
                  <h2>Banco de destino</h2>
                  <p>A restauração é permitida apenas em conexão local.</p>
                </div>
              </div>
              <div className="form-grid">
                <Field label="Perfil local" className="full">
                  <select
                    value={form.profile_id}
                    onChange={(e) =>
                      update("profile_id", Number(e.target.value))
                    }
                  >
                    {profiles.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name} · {item.host}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field
                  label="Banco isolado de destino"
                  hint={
                    profile?.database
                      ? `Use um nome diferente de ${profile.database}.`
                      : "Informe o nome de um banco dedicado à restauração."
                  }
                >
                  <input
                    required
                    pattern="[A-Za-z0-9_]+"
                    value={form.target_database}
                    onChange={(e) => update("target_database", e.target.value)}
                    placeholder="meu_banco_restaurado"
                  />
                </Field>
              </div>
              <div className="create-database">
                <span>O banco ainda não existe?</span>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={
                    !isolated ||
                    !local ||
                    active ||
                    !/^[A-Za-z0-9_]+$/.test(form.target_database)
                  }
                  busy={busy}
                  onClick={() => void createDatabase()}
                >
                  <Database size={16} />
                  Criar banco
                </Button>
              </div>
              <Toggle
                label="Sobrescrever tabelas existentes"
                hint="Remove ou substitui tabelas no destino. Exige confirmação antes de iniciar."
                checked={form.overwrite_tables}
                onChange={(value) => update("overwrite_tables", value)}
              />
            </section>
            <details className="advanced-options">
              <summary>
                Opções avançadas{" "}
                <small>{form.threads || profile?.threads || 8} threads</small>
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
                </Field>{" "}
              </div>
            </details>
            <TableSelector
              controller={restoreTables}
              scope={restoreSelectionKey(form.profile_id, form.backup_dir)}
              mode="restore"
              active={false}
              canQuery={!!form.backup_dir.trim()}
              missingReason="Informe o diretório do backup acima para consultar os arquivos."
              onInvalidQuery={() => backupInput.current?.focus()}
              extra={
                <>
                  <PresetPicker
                    profiles={profiles}
                    disabled={!restoreTables.tables || restoreTables.loading}
                    hint={
                      !restoreTables.tables
                        ? "Consulte as tabelas do backup antes de aplicar uma seleção de origem."
                        : "A seleção pode vir de qualquer perfil de origem; o perfil de destino será mantido."
                    }
                    onApply={(_source, preset) => {
                      const names = new Set(preset.tables);
                      const matches = (restoreTables.tables || []).filter(
                        (table) =>
                          table.database === preset.database &&
                          names.has(table.name),
                      );
                      const found = new Set(matches.map((table) => table.name));
                      const missing = preset.tables.filter(
                        (name) => !found.has(name),
                      );
                      restoreTables.select(matches.map((table) => table.key!));
                      setNotice(
                        `Seleção “${preset.name}” aplicada: ${matches.length} de ${preset.tables.length} tabelas.${missing.length ? ` Ausentes no backup: ${missing.slice(0, 5).join(", ")}${missing.length > 5 ? "…" : ""}.` : ""}`,
                      );
                    }}
                  />
                </>
              }
            />
            {emptySelection && (
              <Alert>
                Nenhuma tabela selecionada. Ajuste a seleção das tabelas do
                backup antes de restaurar.
              </Alert>
            )}
            {!local && (
              <Alert>
                Selecione um perfil com host localhost, 127.0.0.1, ::1 ou
                host.docker.internal para restaurar.
              </Alert>
            )}
            {form.target_database && !isolated && (
              <Alert>
                O banco de destino precisa ser diferente do banco padrão do
                perfil.
              </Alert>
            )}
            {notice && <Alert success>{notice}</Alert>}
            {error && !confirmation && <Alert>{error}</Alert>}
            <div className="form-footer">
              <span>
                {active
                  ? "Aguarde a operação em andamento."
                  : "Validação e execução pelo core local."}
              </span>
              <Button
                type="submit"
                busy={busy}
                disabled={
                  active ||
                  !local ||
                  !isolated ||
                  emptySelection ||
                  restoreTables.loading
                }
              >
                <RotateCcw size={17} />
                Iniciar restauração
              </Button>
            </div>
          </div>
        </form>
      )}
      {confirmation && (
        <Modal
          title="Confirmar restauração"
          onClose={() => {
            if (!busy) setConfirmation(false);
          }}
        >
          <p>Confira o destino e as opções antes de iniciar.</p>
          <dl className="restore-summary">
            <div>
              <dt>Perfil</dt>
              <dd>{profile?.name}</dd>
            </div>
            <div>
              <dt>Servidor</dt>
              <dd>
                {profile?.host}:{profile?.port}
              </dd>
            </div>
            <div>
              <dt>Banco de destino</dt>
              <dd>{form.target_database}</dd>
            </div>
            <div>
              <dt>Threads efetivas</dt>
              <dd>{form.threads || profile?.threads || 8}</dd>
            </div>
            <div className="full">
              <dt>Diretório do backup</dt>
              <dd>
                <code>{form.backup_dir}</code>
              </dd>
            </div>
            <div className="full">
              <dt>Tabelas de origem</dt>
              <dd>
                {restoreTables.references === null
                  ? "Todas as tabelas do backup"
                  : `${restoreTables.references.length} selecionadas`}
                {restoreTables.references && (
                  <ul className="confirmation-tables">
                    {restoreTables.references.slice(0, 8).map((table) => (
                      <li key={JSON.stringify([table.database, table.name])}>
                        {table.database} / {table.name}
                      </li>
                    ))}
                    {restoreTables.references.length > 8 && (
                      <li>
                        Mais {restoreTables.references.length - 8} tabelas…
                      </li>
                    )}
                  </ul>
                )}
              </dd>
            </div>
            <div className="full">
              <dt>Sobrescrever tabelas existentes</dt>
              <dd>{form.overwrite_tables ? "Sim" : "Não"}</dd>
            </div>
          </dl>
          {form.overwrite_tables && (
            <Alert>
              As tabelas existentes no banco de destino poderão ser removidas ou
              substituídas pelos dados do backup.
            </Alert>
          )}
          {error && <Alert>{error}</Alert>}
          <div className="modal-footer">
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => setConfirmation(false)}
            >
              Cancelar
            </Button>
            <Button
              variant={form.overwrite_tables ? "danger" : "primary"}
              busy={busy}
              disabled={
                active ||
                !local ||
                !isolated ||
                emptySelection ||
                restoreTables.loading
              }
              onClick={() => void restore()}
            >
              Confirmar restauração
            </Button>
          </div>
        </Modal>
      )}
    </>
  );
}
