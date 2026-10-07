import { useEffect, useState } from "react";
import { RotateCcw, Database, FolderOpen, RefreshCw } from "lucide-react";
import { api, errorMessage } from "../api";
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
export function Restore({
  profiles,
  backups,
  reloadBackups,
  onJob,
  active,
  onProfiles,
}: {
  profiles: Profile[];
  backups: BackupEntry[];
  reloadBackups: () => Promise<void>;
  onJob: (job: Job) => void;
  active: boolean;
  onProfiles: () => void;
}) {
  const [form, setForm] = useState<RestoreInput>({
    profile_id: profiles[0]?.id || 0,
    backup_dir: "",
    target_database: "",
    threads: 0,
    overwrite_tables: false,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [confirmation, setConfirmation] = useState(false);
  const [confirmedName, setConfirmedName] = useState("");
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
    if (!profiles.some((item) => item.id === form.profile_id))
      setForm((data) => ({ ...data, profile_id: profiles[0]?.id || 0 }));
  }, [profiles]);
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
    try {
      onJob(await api.restore(form));
      setConfirmation(false);
      setConfirmedName("");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    if (form.overwrite_tables) {
      setConfirmedName("");
      setConfirmation(true);
    } else void restore();
  };
  const createDatabase = async () => {
    setBusy(true);
    setError("");
    try {
      onJob(await api.createDatabase(form.profile_id, form.target_database));
      setNotice(
        "Criação do banco iniciada. Acompanhe o resultado nos eventos.",
      );
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
        description="Carregue os dados em um banco local isolado, diferente do banco padrão do perfil."
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
          <div className="card form-card">
            <div className="card-heading">
              <span className="step">01</span>
              <div>
                <h2>Backup de origem</h2>
                <p>Use o catálogo ou informe uma pasta local.</p>
              </div>
            </div>
            <Field label="Backups no diretório padrão">
              <select
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
                  <option key={item.path} value={item.path}>
                    {item.name} · {dateTime(item.modified_at)}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label="Diretório do backup"
              hint="Caminho absoluto da pasta que contém os arquivos e metadata."
            >
              <input
                required
                value={form.backup_dir}
                onChange={(e) => update("backup_dir", e.target.value)}
                placeholder="Caminho absoluto no computador"
              />
            </Field>
            <div className="card-heading section-heading">
              <span className="step">02</span>
              <div>
                <h2>Banco de destino</h2>
                <p>A restauração é permitida apenas em conexão local.</p>
              </div>
            </div>
            <div className="form-grid">
              <Field label="Perfil local" className="full">
                <select
                  value={form.profile_id}
                  onChange={(e) => update("profile_id", Number(e.target.value))}
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
                disabled={active || !local || !isolated}
              >
                <RotateCcw size={17} />
                Iniciar restauração
              </Button>
            </div>
          </div>
          <aside className="card operation-aside">
            <span className="icon-tile amber">
              <FolderOpen size={24} />
            </span>
            <h2>Um destino dedicado</h2>
            <p>
              Restaurar em um banco separado permite inspecionar seus dados
              antes de usá-los.
            </p>
            <div className="aside-rule" />
            <small>DESTINO SELECIONADO</small>
            <strong>{form.target_database || "Não definido"}</strong>
            <code>{profile?.host || "—"}</code>
            <p className="muted">
              O core verifica o caminho do backup e impede restauração no banco
              padrão do perfil.
            </p>
          </aside>
        </form>
      )}
      {confirmation && (
        <Modal
          title="Confirmar sobrescrita"
          onClose={() => {
            if (!busy) setConfirmation(false);
          }}
        >
          <p>
            As tabelas existentes em <strong>{form.target_database}</strong>{" "}
            poderão ser removidas ou substituídas pelos dados do backup.
          </p>
          <Field label="Digite o banco de destino para confirmar">
            <input
              autoFocus
              autoComplete="off"
              value={confirmedName}
              onChange={(e) => setConfirmedName(e.target.value)}
              placeholder={form.target_database}
            />
          </Field>
          {error && <Alert>{error}</Alert>}
          <div className="modal-footer">
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => setConfirmation(false)}
            >
              Voltar
            </Button>
            <Button
              variant="danger"
              busy={busy}
              disabled={confirmedName !== form.target_database || active}
              onClick={() => void restore()}
            >
              Sobrescrever e restaurar
            </Button>
          </div>
        </Modal>
      )}
    </>
  );
}
