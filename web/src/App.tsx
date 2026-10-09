import { useCallback, useEffect, useRef, useState } from "react";
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
import { Alert, Badge, Button, Modal, isActive } from "./components/ui";
import { JobMonitor } from "./components/JobMonitor";
import {
  OperationFeedbackProvider,
  operationNames,
  type OperationFeedback,
  type OperationRunner,
} from "./components/OperationFeedback";
import { ApplicationRestart } from "./components/ApplicationRestart";
import { useJobs } from "./hooks/useJobs";
import { useTheme } from "./hooks/useTheme";
import { Dashboard } from "./pages/Dashboard";
import { Profiles } from "./pages/Profiles";
import { Backup } from "./pages/Backup";
import { Backups } from "./pages/Backups";
import { Databases } from "./pages/Databases";
import { Restore } from "./pages/Restore";
import { History } from "./pages/History";
import { Settings } from "./pages/Settings";
import type {
  BackupEntry,
  BackupSource,
  Diagnostics,
  Job,
  Profile,
  Settings as SettingsMap,
} from "./types";
const mainNavigation = [
  { id: "dashboard", label: "Visão geral", icon: LayoutDashboard },
  { id: "backup", label: "Criar backup", icon: Archive },
  { id: "databases", label: "Bancos disponíveis", icon: Database },
  { id: "restore", label: "Restaurar", icon: RotateCcw },
  { id: "backups", label: "Meus backups", icon: HardDriveDownload },
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
export function App() {
  const { theme, changeTheme } = useTheme();
  const [page, setPage] = useState(getPage);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [monitorVisible, setMonitorVisible] = useState(false);
  const [feedback, setFeedback] = useState<OperationFeedback>();
  const feedbackRequest = useRef<{
    id: number;
    kind: OperationFeedback["kind"];
  } | null>(null);
  const requestSequence = useRef(0);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [settings, setSettings] = useState<SettingsMap>({});
  const [backups, setBackups] = useState<BackupEntry[]>([]);
  const [restorePath, setRestorePath] = useState<string>();
  const [backupSource, setBackupSource] = useState<BackupSource>();
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
  const openedJobs = useRef(new Set<string>());
  const activeJob = jobs.jobs.find((job) => isActive(job.status));
  const selectJob = (job: Job) => {
    feedbackRequest.current = null;
    setFeedback(undefined);
    jobs.select(job);
    setMonitorVisible(true);
  };
  const openMonitor = () => {
    if (feedback) {
      setMonitorVisible(true);
      return;
    }
    const job = activeJob || jobs.selected || jobs.jobs[0] || history[0];
    if (job) selectJob(job);
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
  const runOperation: OperationRunner = async (kind, task, resultMessage) => {
    const id = ++requestSequence.current;
    feedbackRequest.current = { id, kind };
    setFeedback({
      id,
      kind,
      phase: "starting",
      message: "Validando as configurações e preparando o ambiente…",
    });
    setMonitorVisible(true);
    try {
      const result = await task();
      if (feedbackRequest.current?.id === id)
        setFeedback({
          id,
          kind,
          phase: "succeeded",
          message: resultMessage?.(result) || "Operação concluída.",
        });
      return result;
    } catch (error) {
      if (feedbackRequest.current?.id === id)
        setFeedback({
          id,
          kind,
          phase:
            error instanceof DOMException && error.name === "AbortError"
              ? "cancelled"
              : "failed",
          message: errorMessage(error),
        });
      throw error;
    }
  };
  const onJob = (job: Job) => {
    const first = !openedJobs.current.has(job.id);
    const preparing = first && feedbackRequest.current?.kind === job.kind;
    if (preparing) {
      feedbackRequest.current = null;
      setFeedback(undefined);
    }
    jobs.accept(job);
    if (first) {
      openedJobs.current.add(job.id);
      if (!preparing) setMonitorVisible(true);
    }
    setHistory((rows) => [job, ...rows.filter((row) => row.id !== job.id)]);
  };
  const monitorActive = feedback
    ? feedback.phase === "starting"
    : !!jobs.selected && isActive(jobs.selected.status);
  const monitorKind = feedback?.kind || jobs.selected?.kind;
  return (
    <OperationFeedbackProvider value={runOperation}>
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
            </strong>
          </a>
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
              <strong>
                {navigation.find((item) => item.id === page)?.label}
              </strong>
            </div>
            <div className="topbar-status">
              {(feedback || jobs.selected || activeJob) && (
                <Button
                  type="button"
                  variant="secondary"
                  onClick={openMonitor}
                  aria-label="Ver operação"
                >
                  <Terminal size={15} /> Ver operação
                  {activeJob && <Badge status={activeJob.status} />}
                </Button>
              )}
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
                      <small
                        className="topbar-profile-count"
                        aria-hidden="true"
                      >
                        {profiles.length}
                      </small>
                    )}
                  </a>
                ))}
              </nav>
              {!activeJob && (
                <span className="local-status">
                  <span
                    className={apiConnected ? "online-dot" : "offline-dot"}
                  />
                  {apiConnected ? "Core local conectado" : "API desconectada"}
                </span>
              )}
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
                {page === "databases" && (
                  <Databases
                    profiles={profiles}
                    active={!!activeJob}
                    onJob={onJob}
                    onProfiles={() => navigate("profiles")}
                    onBackup={(source) => {
                      setBackupSource(source);
                      navigate("backup");
                    }}
                  />
                )}
                {page === "backup" && (
                  <Backup
                    initialSource={backupSource}
                    onSourceApplied={() => setBackupSource(undefined)}
                    onBrowseDatabases={() => navigate("databases")}
                    profiles={profiles}
                    settings={settings}
                    onJob={onJob}
                    active={!!activeJob}
                    onProfiles={() => navigate("profiles")}
                  />
                )}
                {page === "restore" && (
                  <Restore
                    initialBackup={restorePath}
                    onBackupApplied={() => setRestorePath(undefined)}
                    profiles={profiles}
                    backups={backups}
                    reloadBackups={reloadBackups}
                    onJob={onJob}
                    active={!!activeJob}
                    onProfiles={() => navigate("profiles")}
                  />
                )}
                {page === "backups" && (
                  <Backups
                    backups={backups}
                    reload={reloadBackups}
                    active={!!activeJob}
                    onRestore={(path) => {
                      setRestorePath(path);
                      navigate("restore");
                    }}
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
                    theme={theme}
                    onThemeChange={changeTheme}
                    diagnostics={diagnostics}
                    version={version}
                    onSettings={setSettings}
                    reload={reload}
                    refreshDiagnostics={refreshDiagnostics}
                    maintenanceAction={
                      <ApplicationRestart blocked={Boolean(activeJob)} />
                    }
                  />
                )}
              </>
            )}
            {monitorVisible && monitorKind && (
              <Modal
                title={`${operationNames[monitorKind]} · ${feedback?.phase === "starting" ? "Preparando" : monitorActive ? "Em andamento" : "Resultado"}`}
                onClose={() => setMonitorVisible(false)}
                closeLabel={
                  monitorActive ? "Minimizar operação" : "Fechar resultado"
                }
                className="operation-modal"
              >
                {feedback ? (
                  feedback.phase === "starting" ? (
                    <div
                      className="operation-preparing"
                      role="status"
                      aria-live="polite"
                    >
                      <RefreshCw className="spin" size={24} />
                      <strong>{feedback.message}</strong>
                      <p>
                        O acompanhamento será atualizado assim que o
                        processamento iniciar.
                      </p>
                    </div>
                  ) : (
                    <div
                      className={`operation-result ${feedback.phase === "succeeded" ? "success" : feedback.phase === "failed" ? "error" : "cancelled"}`}
                      role="status"
                    >
                      <h3>
                        {feedback.phase === "failed"
                          ? "Não foi possível concluir a ação"
                          : feedback.phase === "cancelled"
                            ? "Consulta cancelada"
                            : "Operação concluída"}
                      </h3>
                      <p>{feedback.message}</p>
                    </div>
                  )
                ) : (
                  <JobMonitor controller={jobs} />
                )}
                <div className="modal-footer">
                  {!monitorActive && !feedback && jobs.selected && (
                    <Button
                      variant="secondary"
                      onClick={() => {
                        setMonitorVisible(false);
                        navigate(
                          jobs.selected?.kind === "backup"
                            ? "backups"
                            : "history",
                        );
                      }}
                    >
                      {jobs.selected.kind === "backup"
                        ? "Ver backups"
                        : "Ver histórico"}
                    </Button>
                  )}
                  <Button
                    variant={monitorActive ? "secondary" : "primary"}
                    onClick={() => setMonitorVisible(false)}
                  >
                    {monitorActive ? "Minimizar" : "Concluir"}
                  </Button>
                </div>
              </Modal>
            )}
          </main>
        </div>
      </div>
    </OperationFeedbackProvider>
  );
}
