import { useCallback, useEffect, useState } from "react";
import {
  Archive,
  Database,
  HardDriveDownload,
  History as HistoryIcon,
  LayoutDashboard,
  Menu,
  RefreshCw,
  RotateCcw,
  Settings as SettingsIcon,
  Terminal,
  X,
} from "lucide-react";
import { api, errorMessage } from "./api";
import { Alert, Badge, Button, isActive } from "./components/ui";
import { JobMonitor } from "./components/JobMonitor";
import { ApplicationRestart } from "./components/ApplicationRestart";
import { useJobs } from "./hooks/useJobs";
import { Dashboard } from "./pages/Dashboard";
import { Profiles } from "./pages/Profiles";
import { Backup } from "./pages/Backup";
import { Restore } from "./pages/Restore";
import { History } from "./pages/History";
import { Settings } from "./pages/Settings";
import type {
  BackupEntry,
  Diagnostics,
  Job,
  Profile,
  Settings as SettingsMap,
} from "./types";
const mainNavigation = [
  { id: "dashboard", label: "Visão geral", icon: LayoutDashboard },
  { id: "backup", label: "Criar backup", icon: Archive },
  { id: "restore", label: "Restaurar", icon: RotateCcw },
  { id: "history", label: "Histórico", icon: HistoryIcon },
];
const managementNavigation = [
  { id: "profiles", label: "Perfis de banco", icon: Database },
  { id: "settings", label: "Configurações", icon: SettingsIcon },
];
const navigation = [...mainNavigation, ...managementNavigation];
const getPage = () =>
  navigation.some((item) => item.id === location.hash.slice(1))
    ? location.hash.slice(1)
    : "dashboard";
export const monitorPreferenceKey = "dumpersg.showMonitor";
function initialMonitorVisibility() {
  try {
    return localStorage.getItem(monitorPreferenceKey) === "true";
  } catch {
    return false;
  }
}
export function App() {
  const [page, setPage] = useState(getPage);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [monitorVisible, setMonitorVisible] = useState(
    initialMonitorVisibility,
  );
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [settings, setSettings] = useState<SettingsMap>({});
  const [backups, setBackups] = useState<BackupEntry[]>([]);
  const [history, setHistory] = useState<Job[]>([]);
  const [diagnostics, setDiagnostics] = useState<Diagnostics>();
  const [version, setVersion] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [apiConnected, setApiConnected] = useState(false);
  const reloadBackups = useCallback(async () => {
    setBackups(await api.backups());
  }, []);
  const reloadHistory = useCallback(async () => {
    setHistory(await api.history());
  }, []);
  const refreshDiagnostics = useCallback(async () => {
    setDiagnostics(await api.diagnostics());
  }, []);
  const reload = useCallback(async () => {
    const results = await Promise.allSettled([
      api.profiles(),
      api.settings(),
      api.backups(),
      api.history(),
      api.diagnostics(),
      api.health(),
    ]);
    if (results[0].status === "fulfilled") setProfiles(results[0].value);
    if (results[1].status === "fulfilled") setSettings(results[1].value);
    if (results[2].status === "fulfilled") setBackups(results[2].value);
    if (results[3].status === "fulfilled") setHistory(results[3].value);
    if (results[4].status === "fulfilled") setDiagnostics(results[4].value);
    if (results[5].status === "fulfilled") {
      setVersion(results[5].value.version);
      setApiConnected(true);
    } else setApiConnected(false);
    const failure = results.find((result) => result.status === "rejected");
    if (failure?.status === "rejected") throw failure.reason;
  }, []);
  const initialize = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [reload]);
  const onComplete = useCallback(() => {
    void Promise.all([reloadHistory(), reloadBackups()]).catch((e) =>
      setError(errorMessage(e)),
    );
  }, [reloadHistory, reloadBackups]);
  const jobs = useJobs(onComplete);
  const activeJob = jobs.jobs.find((job) => isActive(job.status));
  const showMonitor = (visible: boolean) => {
    setMonitorVisible(visible);
    try {
      localStorage.setItem(monitorPreferenceKey, String(visible));
    } catch {
      /* A escolha continua funcionando quando o armazenamento está bloqueado. */
    }
  };
  const scrollToMonitor = () =>
    requestAnimationFrame(() =>
      document.getElementById("activity-panel")?.scrollIntoView?.({
        block: "nearest",
        behavior: "smooth",
      }),
    );
  const selectJob = (job: Job) => {
    jobs.select(job);
    showMonitor(true);
    scrollToMonitor();
  };
  const toggleMonitor = () => {
    if (monitorVisible) {
      showMonitor(false);
      return;
    }
    const job = jobs.selected || activeJob || jobs.jobs[0] || history[0];
    if (job) selectJob(job);
    else {
      showMonitor(true);
      scrollToMonitor();
    }
  };
  const liveJobs = new Map(jobs.jobs.map((job) => [job.id, job]));
  const liveHistory = history.map((job) => liveJobs.get(job.id) || job);
  const combinedHistory = [
    ...jobs.jobs.filter(
      (job) =>
        isActive(job.status) && !history.some((item) => item.id === job.id),
    ),
    ...liveHistory,
  ];
  useEffect(() => {
    void initialize();
    const change = () => {
      setPage(getPage());
      setMobileOpen(false);
    };
    window.addEventListener("hashchange", change);
    return () => window.removeEventListener("hashchange", change);
  }, [initialize]);
  const navigate = (next: string) => {
    location.hash = next;
    setPage(next);
    setMobileOpen(false);
  };
  const onJob = (job: Job) => {
    jobs.accept(job);
    setHistory((rows) => [job, ...rows.filter((row) => row.id !== job.id)]);
  };
  return (
    <div className="app-shell">
      <a
        className="skip-link"
        href="#main-content"
        onClick={(event) => {
          event.preventDefault();
          document.getElementById("main-content")?.focus();
        }}
      >
        Pular para o conteúdo
      </a>
      {mobileOpen && (
        <button
          className="sidebar-scrim"
          aria-label="Fechar navegação"
          onClick={() => setMobileOpen(false)}
        />
      )}
      <aside className={`sidebar ${mobileOpen ? "open" : ""}`}>
        <a
          className="brand"
          href="#dashboard"
          onClick={() => navigate("dashboard")}
        >
          <span>
            <HardDriveDownload size={22} />
          </span>
          <strong>
            Dumper<span>SG</span>
            <small>DATABASE WORKSPACE</small>
          </strong>
        </a>
        <div className="sidebar-caption">WORKSPACE</div>
        <nav aria-label="Navegação principal">
          {mainNavigation.map((item) => (
            <a
              key={item.id}
              href={`#${item.id}`}
              className={page === item.id ? "active" : ""}
              aria-current={page === item.id ? "page" : undefined}
              onClick={() => navigate(item.id)}
            >
              <item.icon size={19} />
              <span>{item.label}</span>
            </a>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="local-chip">
            <span className={apiConnected ? "online-dot" : "offline-dot"} />
            <div>
              <strong>Ambiente local</strong>
              <small>
                {apiConnected ? "Core conectado" : "Conexão pendente"}
              </small>
            </div>
            <Database size={17} />
          </div>
          <span className="sidebar-version">
            DumperSG {version || "· API v1"}
          </span>
        </div>
      </aside>
      <div className="main-shell">
        <header className="topbar">
          <div className="topbar-location">
            <button
              className="icon-button mobile-menu"
              aria-label={mobileOpen ? "Fechar menu" : "Abrir menu"}
              aria-expanded={mobileOpen}
              onClick={() => setMobileOpen(!mobileOpen)}
            >
              {mobileOpen ? <X size={20} /> : <Menu size={20} />}
            </button>
            <span>Workspace</span>
            <span className="breadcrumb-separator">/</span>
            <strong>
              {navigation.find((item) => item.id === page)?.label}
            </strong>
          </div>
          <div className="topbar-status">
            <Button
              type="button"
              variant="secondary"
              className="monitor-visibility-toggle"
              aria-expanded={monitorVisible}
              aria-controls="activity-panel"
              onClick={toggleMonitor}
            >
              <Terminal size={15} />
              {monitorVisible ? "Ocultar painel" : "Mostrar painel"}
            </Button>
            <nav className="topbar-management" aria-label="Gerenciamento">
              {managementNavigation.map((item) => (
                <a
                  key={item.id}
                  href={`#${item.id}`}
                  className={`button secondary topbar-nav-link ${page === item.id ? "active" : ""}`}
                  aria-label={item.label}
                  aria-current={page === item.id ? "page" : undefined}
                  title={item.label}
                  onClick={() => navigate(item.id)}
                >
                  <item.icon size={15} />
                  <span className="topbar-nav-label">{item.label}</span>
                  {item.id === "profiles" && (
                    <small className="topbar-profile-count" aria-hidden="true">
                      {profiles.length}
                    </small>
                  )}
                </a>
              ))}
            </nav>
            <ApplicationRestart blocked={Boolean(activeJob)} />
            {activeJob ? (
              <button
                className="active-job-link"
                onClick={() => selectJob(activeJob)}
              >
                <Badge status={activeJob.status} />
              </button>
            ) : (
              <span className="local-status">
                <span className={apiConnected ? "online-dot" : "offline-dot"} />
                {apiConnected ? "Core local conectado" : "API desconectada"}
              </span>
            )}
            <span className="avatar" aria-label="Workspace local">
              SG
            </span>
          </div>
        </header>
        <main id="main-content" className="main-content" tabIndex={-1}>
          {error && (
            <div className="load-error">
              <Alert>{error}</Alert>
              <Button
                variant="secondary"
                disabled={loading}
                onClick={() => void initialize()}
              >
                <RefreshCw size={15} />
                Tentar novamente
              </Button>
            </div>
          )}
          {loading ? (
            <div className="loading-state" role="status">
              <RefreshCw className="spin" size={24} />
              <strong>Conectando ao core local…</strong>
              <p>Carregando perfis, configuração e histórico.</p>
            </div>
          ) : (
            <>
              {page === "dashboard" && (
                <Dashboard
                  profiles={profiles}
                  history={combinedHistory}
                  diagnostics={diagnostics}
                  onNavigate={navigate}
                  onSelect={selectJob}
                />
              )}
              {page === "profiles" && (
                <Profiles
                  profiles={profiles}
                  reload={reload}
                  onJob={onJob}
                  active={!!activeJob}
                />
              )}
              {page === "backup" && (
                <Backup
                  profiles={profiles}
                  settings={settings}
                  onJob={onJob}
                  active={!!activeJob}
                  onProfiles={() => navigate("profiles")}
                />
              )}
              {page === "restore" && (
                <Restore
                  profiles={profiles}
                  backups={backups}
                  reloadBackups={reloadBackups}
                  onJob={onJob}
                  active={!!activeJob}
                  onProfiles={() => navigate("profiles")}
                />
              )}
              {page === "history" && (
                <History
                  history={combinedHistory}
                  reload={reloadHistory}
                  onSelect={selectJob}
                  selectedId={jobs.selected?.id}
                />
              )}
              {page === "settings" && (
                <Settings
                  settings={settings}
                  diagnostics={diagnostics}
                  version={version}
                  onSettings={setSettings}
                  reload={reload}
                  refreshDiagnostics={refreshDiagnostics}
                />
              )}
            </>
          )}
          <div id="activity-panel" hidden={!monitorVisible}>
            {monitorVisible && (
              <>
                <JobMonitor controller={jobs} />
                {!jobs.selected && !jobs.error && (
                  <section
                    className="job-monitor empty"
                    aria-label="Acompanhamento da operação"
                  >
                    <Terminal size={22} />
                    <strong>Nenhuma operação selecionada</strong>
                    <p>
                      Inicie uma operação ou selecione os detalhes no histórico
                      para acompanhar o progresso e os logs.
                    </p>
                  </section>
                )}
              </>
            )}
          </div>
          <footer className="main-footer">
            <span>Seus dados permanecem no seu ambiente.</span>
            <span>MYDUMPER + MYLOADER · POWERED BY GO</span>
          </footer>
        </main>
      </div>
    </div>
  );
}
