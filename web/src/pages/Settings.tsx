import { useEffect, useState } from "react";
import {
  CheckCircle2,
  Container,
  Download,
  Palette,
  RefreshCw,
  Save,
  Shield,
} from "lucide-react";
import { api, errorMessage } from "../api";
import { Alert, Button, Field, Modal, PageHeader } from "../components/ui";
import type { Theme } from "../hooks/useTheme";
import type { Diagnostics, Settings as SettingsMap } from "../types";
export function Settings({
  settings,
  theme,
  onThemeChange,
  diagnostics,
  version,
  onSettings,
  reload,
  refreshDiagnostics,
}: {
  settings: SettingsMap;
  theme: Theme;
  onThemeChange: (theme: Theme) => void;
  diagnostics?: Diagnostics;
  version: string;
  onSettings: (data: SettingsMap) => void;
  reload: () => Promise<void>;
  refreshDiagnostics: () => Promise<void>;
}) {
  const [directory, setDirectory] = useState(settings.default_backup_dir || "");
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [confirm, setConfirm] = useState(false);
  useEffect(() => {
    setDirectory(settings.default_backup_dir || "");
  }, [settings.default_backup_dir]);
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    try {
      onSettings(await api.saveSettings({ default_backup_dir: directory }));
      await reload();
      setNotice("Configurações salvas.");
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const diagnose = async () => {
    setBusy(true);
    setError("");
    try {
      await refreshDiagnostics();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const importLegacy = async () => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await api.importLegacy(path);
      await reload();
      setConfirm(false);
      setNotice(
        `Importação concluída: ${result.profiles} perfis e ${result.jobs} operações importados.`,
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
        eyebrow="PREFERÊNCIAS LOCAIS"
        title="Configurações"
        description="Escolha o tema, defina seu diretório padrão, verifique o ambiente e importe dados do aplicativo anterior."
      />
      {notice && <Alert success>{notice}</Alert>}
      {error && !confirm && <Alert>{error}</Alert>}
      <div className="settings-layout">
        <section className="card form-card full">
          <div className="card-heading">
            <span className="icon-tile small">
              <Palette size={19} />
            </span>
            <div>
              <h2>Aparência</h2>
              <p>Escolha como o DumperSG aparece neste navegador.</p>
            </div>
          </div>
          <Field
            label="Tema da interface"
            hint="Aplicado imediatamente e salvo automaticamente neste navegador."
          >
            <select
              value={theme}
              onChange={(event) => onThemeChange(event.target.value as Theme)}
            >
              <option value="light">Claro</option>
              <option value="dark">Escuro</option>
            </select>
          </Field>
        </section>
        <section className="card form-card">
          <div className="card-heading">
            <span className="icon-tile small">
              <Save size={19} />
            </span>
            <div>
              <h2>Armazenamento</h2>
              <p>O diretório usado quando o destino do backup estiver vazio.</p>
            </div>
          </div>
          <form onSubmit={save}>
            <Field
              label="Diretório padrão de backups"
              hint="Use um caminho absoluto acessível pelo core local."
            >
              <input
                value={directory}
                onChange={(e) => setDirectory(e.target.value)}
                placeholder="Caminho absoluto no computador"
              />
            </Field>
            <div className="form-footer">
              <span>Também usado pelo catálogo de restauração.</span>
              <Button type="submit" busy={busy}>
                <Save size={16} />
                Salvar
              </Button>
            </div>
          </form>
        </section>
        <section className="card form-card">
          <div className="card-heading">
            <span className="icon-tile small purple">
              <Container size={20} />
            </span>
            <div>
              <h2>Ambiente Docker</h2>
              <p>Necessário para executar mydumper e myloader.</p>
            </div>
          </div>
          <div
            className={`diagnostic ${diagnostics?.available ? "available" : ""}`}
          >
            <span className="diagnostic-dot" />
            <div>
              <strong>
                {diagnostics
                  ? diagnostics.available
                    ? "Docker disponível"
                    : "Docker indisponível"
                  : "Diagnóstico pendente"}
              </strong>
              <p>
                {diagnostics?.message ||
                  "Atualize o diagnóstico para verificar o ambiente."}
              </p>
              {diagnostics?.version && <code>{diagnostics.version}</code>}
            </div>
          </div>
          <Button
            variant="secondary"
            busy={busy}
            onClick={() => void diagnose()}
          >
            <RefreshCw size={16} />
            Verificar novamente
          </Button>
        </section>
        <section className="card form-card legacy-card">
          <div className="card-heading">
            <span className="icon-tile small amber">
              <Download size={19} />
            </span>
            <div>
              <h2>Importar aplicativo anterior</h2>
              <p>Traga perfis e histórico do SQLite legado do DumperSG.</p>
            </div>
          </div>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              setError("");
              setConfirm(true);
            }}
          >
            <Field
              label="Caminho do SQLite legado"
              hint="A importação preserva o arquivo original e mantém perfis existentes com o mesmo nome."
            >
              <input
                required
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="Caminho absoluto do arquivo .db"
              />
            </Field>
            <div className="form-footer">
              <span>A importação só ocorre quando você confirmar.</span>
              <Button
                type="submit"
                variant="secondary"
                disabled={busy || !path.trim()}
              >
                <Download size={16} />
                Importar legado
              </Button>
            </div>
          </form>
        </section>
        <section className="card about-card">
          <Shield size={25} />
          <h2>Executado na sua máquina</h2>
          <p>
            API em loopback, token de sessão nas alterações e credenciais
            mantidas pelo backend local.
          </p>
          <div>
            <CheckCircle2 size={15} />
            Senhas nunca são exibidas pela API
          </div>
          <small>DumperSG {version || "· core local"} · API v1</small>
        </section>
      </div>
      {confirm && (
        <Modal
          title="Importar dados legados"
          onClose={() => {
            if (!busy) setConfirm(false);
          }}
        >
          <p>Importar perfis e operações deste arquivo SQLite?</p>
          <code className="path-block">{path}</code>
          <p className="muted">
            O arquivo original será preservado. Perfis que já existem com o
            mesmo nome serão mantidos.
          </p>
          {error && <Alert>{error}</Alert>}
          <div className="modal-footer">
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => setConfirm(false)}
            >
              Voltar
            </Button>
            <Button busy={busy} onClick={() => void importLegacy()}>
              Confirmar importação
            </Button>
          </div>
        </Modal>
      )}
    </>
  );
}
