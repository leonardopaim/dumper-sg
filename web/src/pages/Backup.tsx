import { useEffect, useState } from "react";
import {
  Archive,
  ArrowRight,
  FolderOpen,
  SlidersHorizontal,
} from "lucide-react";
import { api, errorMessage } from "../api";
import {
  Alert,
  Button,
  Empty,
  Field,
  PageHeader,
  Toggle,
} from "../components/ui";
import type { BackupInput, Job, Profile, Settings } from "../types";
export function Backup({
  profiles,
  settings,
  active,
  onJob,
  onProfiles,
}: {
  profiles: Profile[];
  settings: Settings;
  active: boolean;
  onJob: (job: Job) => void;
  onProfiles: () => void;
}) {
  const [form, setForm] = useState<BackupInput>({
    profile_id: profiles[0]?.id || 0,
    database: profiles[0]?.database || "",
    destination_dir: "",
    threads: 0,
    compress: true,
    ssl: profiles[0]?.ssl || false,
    non_locking: true,
    ignore_regex: "",
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const profile = profiles.find((item) => item.id === form.profile_id);
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
    if (!profiles.some((item) => item.id === form.profile_id))
      selectProfile(profiles[0]?.id || 0);
  }, [profiles]);
  const update = <K extends keyof BackupInput>(key: K, value: BackupInput[K]) =>
    setForm((data) => ({ ...data, [key]: value }));
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    setBusy(true);
    try {
      onJob(await api.backup(form));
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
        description="Exporte seu banco com mydumper e acompanhe cada etapa em tempo real."
      />
      {profiles.length === 0 ? (
        <Empty title="Nenhum perfil de conexão">
          <Button onClick={onProfiles}>Configurar um perfil</Button>
        </Empty>
      ) : (
        <form onSubmit={submit} className="operation-layout">
          <div className="card form-card">
            <div className="card-heading">
              <span className="step">01</span>
              <div>
                <h2>Origem e destino</h2>
                <p>Selecione o banco e onde guardar os arquivos.</p>
              </div>
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
              <Field label="Banco de origem" className="full">
                <input
                  required
                  value={form.database}
                  onChange={(e) => update("database", e.target.value)}
                  placeholder="nome_do_banco"
                />
              </Field>
              <Field
                label="Diretório de destino"
                className="full"
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
            <div className="card-heading section-heading">
              <span className="step">02</span>
              <div>
                <h2>Opções de exportação</h2>
                <p>Ajuste o processamento para este backup.</p>
              </div>
              <SlidersHorizontal size={18} />
            </div>
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
            {error && <Alert>{error}</Alert>}
            <div className="form-footer">
              <span>
                {active
                  ? "Aguarde a operação em andamento."
                  : "O backup continua ao navegar entre telas."}
              </span>
              <Button type="submit" disabled={active} busy={busy}>
                <Archive size={17} />
                Iniciar backup
                <ArrowRight size={16} />
              </Button>
            </div>
          </div>
          <aside className="card operation-aside">
            <span className="icon-tile">
              <FolderOpen size={24} />
            </span>
            <h2>Arquivos sob seu controle</h2>
            <p>
              O core executa o mydumper em Docker e salva o resultado no
              diretório local informado.
            </p>
            <div className="aside-rule" />
            <small>ORIGEM SELECIONADA</small>
            <strong>{profile?.name || "—"}</strong>
            <code>
              {profile?.host}:{profile?.port}
            </code>
            <p className="muted">
              Após concluir, o backup aparece no catálogo de restauração se
              estiver no diretório padrão.
            </p>
          </aside>
        </form>
      )}
    </>
  );
}
