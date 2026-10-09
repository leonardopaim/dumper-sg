import { useOperationRequest } from "../components/OperationFeedback";
import { useState } from "react";
import { Edit3, Plus, Trash2, PlugZap } from "lucide-react";
import { ProfilePresets } from "../components/ProfilePresets";
import { api, errorMessage } from "../api";
import {
  Alert,
  Button,
  Empty,
  Field,
  Modal,
  PageHeader,
  Toggle,
} from "../components/ui";
import type { Profile, ProfileInput, Job } from "../types";
const initial: ProfileInput = {
  name: "",
  host: "127.0.0.1",
  port: 3306,
  user: "root",
  database: "",
  ssl: false,
  threads: 8,
};
export function Profiles({
  profiles,
  reload,
  onJob,
  active,
}: {
  profiles: Profile[];
  reload: () => Promise<void>;
  onJob: (job: Job) => void;
  active: boolean;
}) {
  const runOperation = useOperationRequest();
  const [editing, setEditing] = useState<Profile | "new" | null>(null);
  const [managing, setManaging] = useState<Profile | null>(null);
  const [removing, setRemoving] = useState<Profile | null>(null);
  const [form, setForm] = useState<ProfileInput>(initial);
  const [passwordMode, setPasswordMode] = useState<
    "keep" | "replace" | "clear"
  >("keep");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const open = (profile: Profile | "new") => {
    setError("");
    setNotice("");
    setEditing(profile);
    setPasswordMode(profile === "new" ? "replace" : "keep");
    setForm(
      profile === "new"
        ? { ...initial, password: "" }
        : {
            name: profile.name,
            host: profile.host,
            port: profile.port,
            user: profile.user,
            database: profile.database,
            ssl: profile.ssl,
            threads: profile.threads,
          },
    );
  };
  const update = <K extends keyof ProfileInput>(
    key: K,
    value: ProfileInput[K],
  ) => setForm((data) => ({ ...data, [key]: value }));
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const data = { ...form };
      if (passwordMode === "keep") delete data.password;
      if (passwordMode === "clear") data.password = "";
      if (editing === "new") await api.createProfile(data);
      else if (editing) await api.updateProfile(editing.id, data);
      setEditing(null);
      setForm(initial);
      await reload();
      setNotice("Perfil salvo.");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    if (!removing) return;
    setBusy(true);
    setError("");
    try {
      await api.deleteProfile(removing.id);
      setRemoving(null);
      await reload();
      setNotice("Perfil excluído.");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const test = async (profile: Profile) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      onJob(
        await runOperation("connection_test", () =>
          api.testConnection(profile.id),
        ),
      );
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <PageHeader
        eyebrow="CONEXÕES"
        title="Perfis de banco"
        description="Gerencie e teste suas conexões."
        action={
          <Button onClick={() => open("new")}>
            <Plus size={17} />
            Novo perfil
          </Button>
        }
      />
      {notice && <Alert success>{notice}</Alert>}
      {error && !editing && !removing && <Alert>{error}</Alert>}
      {profiles.length === 0 ? (
        <Empty title="Seu primeiro banco começa aqui">
          Crie um perfil com os dados de conexão para fazer backups e
          restaurações.
        </Empty>
      ) : (
        <section className="card profile-list table-scroll">
          <table>
            <caption className="sr-only">Perfis de conexão</caption>
            <thead>
              <tr>
                <th>Perfil</th>
                <th>Servidor</th>
                <th>Banco padrão</th>
                <th>Ações</th>
              </tr>
            </thead>
            <tbody>
              {profiles.map((profile) => (
                <tr key={profile.id}>
                  <td>
                    <strong>{profile.name}</strong>
                  </td>
                  <td>
                    {profile.host}:{profile.port}
                  </td>
                  <td>{profile.database || "Não definido"}</td>
                  <td>
                    <div className="profile-row-actions">
                      <Button
                        variant="secondary"
                        disabled={active || busy}
                        onClick={() => void test(profile)}
                      >
                        <PlugZap size={15} /> Testar
                      </Button>
                      <button
                        className="icon-button"
                        aria-label={`Editar ${profile.name}`}
                        onClick={() => open(profile)}
                      >
                        <Edit3 size={17} />
                      </button>
                      <details className="row-actions">
                        <summary
                          aria-label={`Mais ações de ${profile.name}`}
                          role="button"
                        >
                          •••
                        </summary>
                        <div>
                          <Button
                            variant="ghost"
                            onClick={() => setManaging(profile)}
                          >
                            Seleções de tabelas
                          </Button>
                          <Button
                            variant="danger"
                            aria-label={`Excluir ${profile.name}`}
                            disabled={active || busy}
                            onClick={() => {
                              setRemoving(profile);
                              setError("");
                            }}
                          >
                            <Trash2 size={15} /> Excluir
                          </Button>
                        </div>
                      </details>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      {managing && (
        <ProfilePresets
          profile={managing}
          active={active}
          onJob={onJob}
          reload={reload}
          onClose={() => setManaging(null)}
        />
      )}
      {editing && (
        <Modal
          title={editing === "new" ? "Novo perfil" : `Editar ${editing.name}`}
          onClose={() => {
            if (!busy) {
              setEditing(null);
              setForm(initial);
            }
          }}
        >
          <form onSubmit={save}>
            {editing === "new" && (
              <p className="table-hint">
                Salve a conexão primeiro para consultar e criar seleções de
                tabelas neste perfil.
              </p>
            )}
            {error && <Alert>{error}</Alert>}
            <div className="form-grid">
              <Field label="Nome do perfil" className="full">
                <input
                  autoFocus
                  required
                  maxLength={120}
                  value={form.name}
                  onChange={(e) => update("name", e.target.value)}
                  placeholder="Ex.: Produção · Financeiro"
                />
              </Field>
              <Field label="Host">
                <input
                  required
                  value={form.host}
                  onChange={(e) => update("host", e.target.value)}
                  placeholder="127.0.0.1"
                />
              </Field>
              <Field label="Porta">
                <input
                  type="number"
                  min={1}
                  max={65535}
                  required
                  value={form.port}
                  onChange={(e) => update("port", Number(e.target.value))}
                />
              </Field>
              <Field label="Usuário">
                <input
                  required
                  autoComplete="off"
                  value={form.user}
                  onChange={(e) => update("user", e.target.value)}
                />
              </Field>
              <Field label="Banco padrão">
                <input
                  value={form.database}
                  onChange={(e) => update("database", e.target.value)}
                />
              </Field>
              {editing !== "new" && (
                <Field
                  label="Senha"
                  className="full"
                  hint={
                    editing.has_password
                      ? "Há uma senha configurada. O valor nunca é retornado pela API."
                      : "Este perfil não tem senha configurada."
                  }
                >
                  <select
                    value={passwordMode}
                    onChange={(e) => {
                      setPasswordMode(e.target.value as typeof passwordMode);
                      update("password", "");
                    }}
                  >
                    <option value="keep">Manter senha atual</option>
                    <option value="replace">Definir nova senha</option>
                    <option value="clear">Limpar senha</option>
                  </select>
                </Field>
              )}
              {passwordMode === "replace" && (
                <Field
                  label={editing === "new" ? "Senha" : "Nova senha"}
                  className="full"
                  hint="Armazenada somente pelo backend local."
                >
                  <input
                    type="password"
                    autoComplete="new-password"
                    value={form.password || ""}
                    onChange={(e) => update("password", e.target.value)}
                  />
                </Field>
              )}
              <details className="advanced-options full">
                <summary>Opções avançadas</summary>
                <div className="form-grid">
                  <Field label="Threads padrão">
                    <input
                      type="number"
                      min={1}
                      max={128}
                      required
                      value={form.threads}
                      onChange={(e) =>
                        update("threads", Number(e.target.value))
                      }
                    />
                  </Field>
                  <Toggle
                    label="Usar SSL"
                    hint="Conexão criptografada."
                    checked={form.ssl}
                    onChange={(value) => update("ssl", value)}
                  />
                </div>
              </details>
            </div>
            <div className="modal-footer">
              <Button
                type="button"
                variant="secondary"
                disabled={busy}
                onClick={() => {
                  setEditing(null);
                  setForm(initial);
                }}
              >
                Cancelar
              </Button>
              <Button type="submit" busy={busy}>
                Salvar perfil
              </Button>
            </div>
          </form>
        </Modal>
      )}
      {removing && (
        <Modal
          title="Excluir perfil"
          onClose={() => {
            if (!busy) setRemoving(null);
          }}
        >
          <p>
            Excluir <strong>{removing.name}</strong>? O histórico de operações
            será mantido.
          </p>
          {error && <Alert>{error}</Alert>}
          <div className="modal-footer">
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => setRemoving(null)}
            >
              Voltar
            </Button>
            <Button variant="danger" busy={busy} onClick={() => void remove()}>
              Excluir perfil
            </Button>
          </div>
        </Modal>
      )}
    </>
  );
}
