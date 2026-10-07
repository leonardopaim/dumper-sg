import { useCallback, useEffect, useRef, useState } from "react";
import { api, errorMessage } from "../api";
import { isActive } from "../components/ui";
import type { Job, JobEvent } from "../types";
export function useJobs(onComplete: () => void) {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [error, setError] = useState("");
  const [pollError, setPollError] = useState("");
  const [eventsLoading, setEventsLoading] = useState(false);
  const [cancelBusy, setCancelBusy] = useState(false);
  const selection = useRef<string | null>(null);
  const cursor = useRef(0);
  const previous = useRef(new Map<string, Job>());
  const completion = useRef(onComplete);
  completion.current = onComplete;
  const refresh = useCallback(async () => {
    const rows = await api.jobs();
    let finished = false;
    for (const job of rows) {
      const old = previous.current.get(job.id);
      if (old && isActive(old.status) && !isActive(job.status)) finished = true;
    }
    previous.current = new Map(rows.map((job) => [job.id, job]));
    setJobs(rows);
    if (!selection.current) {
      const current = rows.find(
        (job) => isActive(job.status) || job.cleanup_required,
      );
      if (current) {
        selection.current = current.id;
        cursor.current = 0;
        setEventsLoading(true);
        setSelectedId(current.id);
      }
    }
    if (finished) completion.current();
    return rows;
  }, []);
  const select = useCallback((job: Job) => {
    if (selection.current === job.id) return;
    selection.current = job.id;
    cursor.current = 0;
    setEvents([]);
    setError("");
    setPollError("");
    setEventsLoading(true);
    setSelectedId(job.id);
    setJobs((rows) =>
      rows.some((row) => row.id === job.id) ? rows : [job, ...rows],
    );
  }, []);
  const accept = useCallback(
    (job: Job) => {
      previous.current.set(job.id, job);
      setJobs((rows) => [job, ...rows.filter((row) => row.id !== job.id)]);
      select(job);
    },
    [select],
  );
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      let rows: Job[] = [];
      try {
        rows = await refresh();
        if (!disposed) setPollError("");
      } catch (e) {
        if (!disposed) setPollError(errorMessage(e));
      }
      const id = selection.current;
      if (id && !disposed) {
        try {
          const [job, incoming] = await Promise.all([
            api.job(id),
            api.events(id, cursor.current),
          ]);
          if (!disposed && selection.current === id) {
            setJobs((list) =>
              list.some((row) => row.id === id)
                ? list.map((row) => (row.id === id ? job : row))
                : [job, ...list],
            );
            if (incoming.length) {
              cursor.current = Math.max(
                cursor.current,
                ...incoming.map((event) => event.sequence),
              );
              setEvents((list) =>
                [
                  ...list,
                  ...incoming.filter(
                    (event) =>
                      !list.some((item) => item.sequence === event.sequence),
                  ),
                ].slice(-2000),
              );
            }
            setEventsLoading(false);
          }
        } catch (e) {
          if (!disposed && selection.current === id) {
            setPollError(errorMessage(e));
            setEventsLoading(false);
          }
        }
      }
      if (!disposed)
        timer = setTimeout(
          poll,
          rows.some((row) => isActive(row.status)) || id ? 1200 : 4000,
        );
    }
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [refresh, selectedId]);
  const cancel = async (job: Job) => {
    setCancelBusy(true);
    setError("");
    try {
      const updated = await api.cancel(job.id);
      setJobs((rows) =>
        rows.map((row) => (row.id === updated.id ? updated : row)),
      );
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setCancelBusy(false);
    }
  };
  return {
    jobs,
    selected: jobs.find((job) => job.id === selectedId),
    events,
    error: error || pollError,
    eventsLoading,
    cancelBusy,
    select,
    accept,
    cancel,
    refresh,
  };
}
export type JobsController = ReturnType<typeof useJobs>;
