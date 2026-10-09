import { useEffect, useRef, useState } from "react";
import {
  Activity,
  ChevronDown,
  ChevronUp,
  Terminal,
  Square,
} from "lucide-react";
import { Badge, Button, Alert, dateTime, isActive, kinds } from "./ui";
import type { JobsController } from "../hooks/useJobs";

export const logsPreferenceKey = "dumpersg.showLogs";
function initialLogsPreference() {
  try {
    return localStorage.getItem(logsPreferenceKey) === "true";
  } catch {
    return false;
  }
}
function elapsed(start: string, finish?: string) {
  const seconds = Math.max(
    0,
    Math.floor(
      ((finish ? Date.parse(finish) : Date.now()) - Date.parse(start)) / 1000,
    ),
  );
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return minutes < 60
    ? `${minutes}min ${seconds % 60}s`
    : `${Math.floor(minutes / 60)}h ${minutes % 60}min`;
}

export function JobMonitor({ controller }: { controller: JobsController }) {
  const {
    selected: job,
    events,
    error,
    eventsLoading,
    cancel,
    cancelBusy,
  } = controller;
  const [expanded, setExpanded] = useState(initialLogsPreference);
  const [autoScroll, setAutoScroll] = useState(true);
  const logRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (autoScroll && logRef.current)
      logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [events, autoScroll, expanded]);
  const toggleLogs = () => {
    const next = !expanded;
    setExpanded(next);
    try {
      localStorage.setItem(logsPreferenceKey, String(next));
    } catch {
      /* A preferência em memória continua funcionando sem armazenamento. */
    }
  };
  if (!job) return error ? <Alert>{error}</Alert> : null;
  const active = isActive(job.status);
  const progress = Math.min(100, Math.max(0, job.progress));
  const indeterminate = active && progress === 0;
  const logPanelId = `job-logs-${job.id}`;
  return (
    <section
      id="job-monitor"
      className={`job-monitor job-${job.kind}`}
      aria-label="Acompanhamento da operação"
    >
      <div className="monitor-heading">
        <div className="monitor-title">
          <span className="icon-tile small">
            <Activity size={19} />
          </span>
          <div>
            <strong>
              {kinds[job.kind]}{" "}
              <span className="muted">/ {job.profile_name}</span>
            </strong>
            <small>
              {job.database || "Conexão"} · Início {dateTime(job.started_at)} ·
              Duração {elapsed(job.started_at, job.finished_at)}
            </small>
          </div>
        </div>
        <div className="inline-actions">
          <Badge status={job.status} />
          {(active || job.cleanup_required) && (
            <Button
              variant="danger"
              busy={cancelBusy}
              disabled={job.status === "cancel_requested"}
              onClick={() => void cancel(job)}
            >
              <Square size={14} />
              {job.cleanup_required ? "Verificar término" : "Cancelar"}
            </Button>
          )}
          <Button
            variant="secondary"
            className="monitor-log-toggle"
            aria-expanded={expanded}
            aria-controls={logPanelId}
            onClick={toggleLogs}
          >
            <Terminal size={14} />
            {expanded ? "Ocultar logs" : "Mostrar logs"}
            {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
          </Button>
        </div>
      </div>
      <div className="monitor-progress">
        <div
          className={`monitor-message ${job.status === "failed" ? "failed" : ""}`}
        >
          <span>{job.message || "Aguardando eventos da operação…"}</span>
          <strong>
            {indeterminate
              ? "Em andamento · sem estimativa"
              : `${progress}% estimado`}
            {job.exit_code !== undefined && (
              <small> · saída {job.exit_code}</small>
            )}
          </strong>
        </div>
        <div
          className={`progress-track ${indeterminate ? "indeterminate" : ""}`}
          role="progressbar"
          aria-valuenow={indeterminate ? undefined : progress}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={`Progresso estimado de ${kinds[job.kind]}`}
          aria-valuetext={
            indeterminate
              ? "Em andamento, sem estimativa"
              : `${progress}% estimado`
          }
        >
          <span style={indeterminate ? undefined : { width: `${progress}%` }} />
        </div>
      </div>
      {job.path && (
        <div className="monitor-path">
          <span>Caminho</span>
          <code>{job.path}</code>
        </div>
      )}
      <div id={logPanelId} hidden={!expanded}>
        {expanded && (
          <>
            <div className="log-toolbar">
              <span>
                <Terminal size={14} /> Eventos do processo
                {events.length > 0 && events[0].sequence > 1 && (
                  <small> · Eventos anteriores fora da retenção</small>
                )}
              </span>
              <div className="log-toolbar-actions">
                <label>
                  <input
                    type="checkbox"
                    checked={autoScroll}
                    onChange={(e) => setAutoScroll(e.target.checked)}
                  />{" "}
                  Seguir logs
                </label>
                <Button
                  type="button"
                  variant="ghost"
                  className="log-close"
                  aria-controls={logPanelId}
                  onClick={toggleLogs}
                >
                  <ChevronUp size={14} /> Fechar logs
                </Button>
              </div>
            </div>
            <div
              className="logs"
              ref={logRef}
              tabIndex={0}
              aria-label="Logs da operação"
            >
              {events.length === 0 ? (
                <p className="log-empty">
                  {eventsLoading
                    ? "Carregando eventos…"
                    : active
                      ? "Aguardando os primeiros eventos…"
                      : "Não há eventos disponíveis em memória. O resultado permanece no histórico."}
                </p>
              ) : (
                events.map((event) => (
                  <div
                    className={`log-line ${event.level.toLowerCase()}`}
                    key={event.sequence}
                  >
                    <time>
                      {new Date(event.time).toLocaleTimeString("pt-BR")}
                    </time>
                    <span className="log-level">{event.level}</span>
                    <span>{event.message}</span>
                  </div>
                ))
              )}
            </div>
          </>
        )}
      </div>
      {job.cleanup_required && (
        <Alert>
          O término do container ainda não foi confirmado pelo daemon Docker no
          WSL. Verifique o container da operação e use “Verificar término” para
          liberar o core.
        </Alert>
      )}
      {error && <Alert>{error}</Alert>}
    </section>
  );
}
