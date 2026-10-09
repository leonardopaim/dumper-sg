import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import { boundedRequest, waitForRestart } from "./application";
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});
describe("reconexão após reinício", () => {
  it("não confunde uma resposta da instância antiga com sucesso", async () => {
    vi.useFakeTimers();
    const get = vi
      .spyOn(api, "application")
      .mockResolvedValue({ instance_id: "old", restart_available: true });
    const finished = vi.fn();
    const result = waitForRestart("old", new AbortController().signal).then(
      finished,
    );
    await vi.advanceTimersByTimeAsync(1000);
    expect(finished).not.toHaveBeenCalled();
    get.mockResolvedValue({ instance_id: "new", restart_available: true });
    await vi.advanceTimersByTimeAsync(500);
    await result;
    expect(finished).toHaveBeenCalledTimes(1);
  });
  it("limita a espera se o backend não voltar", async () => {
    vi.useFakeTimers();
    vi.spyOn(api, "application").mockRejectedValue(new TypeError("offline"));
    const result = waitForRestart("old", new AbortController().signal, 1000);
    const checked = expect(result).rejects.toThrow(/restart.ps1 -Force/);
    await vi.advanceTimersByTimeAsync(1000);
    await checked;
  });
  it("limita requisições que não respondem", async () => {
    vi.useFakeTimers();
    const result = boundedRequest(
      () => new Promise(() => undefined),
      new AbortController().signal,
    );
    const checked = expect(result).rejects.toMatchObject({
      name: "AbortError",
    });
    await vi.advanceTimersByTimeAsync(3000);
    await checked;
  });
});
