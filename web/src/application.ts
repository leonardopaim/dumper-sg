import { api } from "./api";

export async function boundedRequest<T>(
  run: (signal: AbortSignal) => Promise<T>,
  parent: AbortSignal,
): Promise<T> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  parent.addEventListener("abort", abort, { once: true });
  if (parent.aborted) controller.abort();
  const timer = setTimeout(abort, 3000);
  try {
    return await new Promise<T>((resolve, reject) => {
      const cancelled = () =>
        reject(new DOMException("Tempo de resposta excedido.", "AbortError"));
      if (controller.signal.aborted) {
        cancelled();
        return;
      }
      controller.signal.addEventListener("abort", cancelled, { once: true });
      run(controller.signal)
        .then(resolve, reject)
        .finally(() =>
          controller.signal.removeEventListener("abort", cancelled),
        );
    });
  } finally {
    clearTimeout(timer);
    parent.removeEventListener("abort", abort);
  }
}

export async function waitForRestart(
  previous: string,
  signal: AbortSignal,
  timeoutMs = 45000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    signal.throwIfAborted();
    try {
      const current = await boundedRequest(api.application, signal);
      if (current.instance_id && current.instance_id !== previous) return;
    } catch {
      signal.throwIfAborted();
    }
    await new Promise<void>((resolve, reject) => {
      const abort = () => {
        clearTimeout(timer);
        reject(new DOMException("Reinício cancelado.", "AbortError"));
      };
      const timer = setTimeout(() => {
        signal.removeEventListener("abort", abort);
        resolve();
      }, 500);
      signal.addEventListener("abort", abort, { once: true });
    });
  }
  throw new Error(
    "A aplicação não voltou a responder. Use o comando .\\scripts\\restart.ps1 -Force se o backend estiver travado.",
  );
}
