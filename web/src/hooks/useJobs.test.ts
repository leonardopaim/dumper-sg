import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useJobs } from "./useJobs";
import { api } from "../api";
import type { Job } from "../types";
const running: Job = {
  id: "job1",
  kind: "backup",
  status: "running",
  profile_id: 1,
  profile_name: "Local",
  database: "origem",
  path: "",
  started_at: "2026-10-07T12:00:00Z",
  progress: 2,
  message: "Executando",
};
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});
describe("acompanhamento de jobs", () => {
  it("seleciona histórico e retém eventos incrementais ao consultar novamente", async () => {
    vi.spyOn(api, "jobs").mockResolvedValue([running]);
    vi.spyOn(api, "job").mockResolvedValue(running);
    const events = vi
      .spyOn(api, "events")
      .mockResolvedValue([
        {
          sequence: 5,
          time: running.started_at,
          level: "info",
          message: "Exportando tabelas",
        },
      ]);
    const { result, rerender } = renderHook(() => useJobs(vi.fn()));
    await waitFor(() => expect(result.current.jobs).toHaveLength(1));
    act(() => result.current.select(running));
    await waitFor(() => expect(result.current.events).toHaveLength(1));
    expect(events).toHaveBeenCalledWith("job1", 0);
    rerender();
    expect(result.current.selected?.id).toBe("job1");
    expect(result.current.events[0].message).toBe("Exportando tabelas");
    await waitFor(() => expect(events).toHaveBeenCalledWith("job1", 5), {
      timeout: 3000,
    });
    expect(result.current.events).toHaveLength(1);
  });
});
