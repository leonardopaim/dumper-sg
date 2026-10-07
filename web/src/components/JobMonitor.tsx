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
export function JobMonitor({ controller }: { controller: JobsController }) {
  const {
    selected: job,
    events,
    error,
    eventsLoading,
    cancel,
    cancelBusy,
  } = controller;
  const [expanded, setExpanded] = useState(true);
  const [autoScroll, setAutoScroll] = useState(true);
  const logRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (autoScroll && logRef.current)
      logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [events, autoScroll, expanded]);
  if (!job) return error ? <Alert>{error}</Alert> : null;
  const active = isActive(job.status);
  return (
    <section className="job-monitor" aria-label="Acompanhamento da operação">
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
              {job.database || "Conexão"} · {dateTime(job.started_at)}
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
          <button
            className="icon-button"
            aria-label={expanded ? "Recolher logs" : "Expandir logs"}
            aria-expanded={expanded}
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? <ChevronUp size={20} /> : <ChevronDown size={20} />}
          </button>
        </div>
      </div>
      <div
        className="progress-track"
        role="progressbar"
        aria-valuenow={job.progress}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label="Progresso estimado"
      >
        <span
          style={{ width: `${Math.min(100, Math.max(0, job.progress))}%` }}
        />
      </div>
      <div className="monitor-message">
        <span>{job.message || "Aguardando eventos da operação…"}</span>
        <small>
          {job.progress}% estimado
          {job.exit_code !== undefined && ` · saída ${job.exit_code}`}
        </small>
      </div>
      {expanded && (
        <>
          <div className="log-toolbar">
            <span>
              <Terminal size={14} /> Eventos do processo
              {events.length > 0 && events[0].sequence > 1 && (
                <small> · Eventos anteriores fora da retenção</small>
              )}
            </span>
            <label>
              <input
                type="checkbox"
                checked={autoScroll}
                onChange={(e) => setAutoScroll(e.target.checked)}
              />{" "}
              Seguir logs
            </label>
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
          {job.path && (
            <div className="monitor-path">
              <span>Caminho</span>
              <code>{job.path}</code>
            </div>
          )}
        </>
      )}
      {job.cleanup_required && (
        <Alert>
          O término do container Docker ainda não foi confirmado. Verifique o
          container da operação no Docker Desktop e use “Verificar término” para
          liberar o core.
        </Alert>
      )}
      {error && <Alert>{error}</Alert>}
    </section>
  );
}
