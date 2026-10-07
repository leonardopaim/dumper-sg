import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const fetchMock = vi.fn();
const response = (body: unknown, status = 200) =>
  new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
beforeEach(() => {
  vi.resetModules();
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => vi.unstubAllGlobals());
describe("API local", () => {
  it("obtém a sessão uma vez e envia token nas mutações, sem colocar token na URL", async () => {
    fetchMock
      .mockResolvedValueOnce(response({ token: "session-secret" }))
      .mockResolvedValueOnce(response({ id: 4 }))
      .mockResolvedValueOnce(response(null, 204));
    const { api } = await import("./api");
    await api.createProfile({
      name: "Local",
      host: "127.0.0.1",
      port: 3306,
      user: "root",
      password: "test-password",
      database: "db",
      ssl: false,
      threads: 8,
    });
    await api.deleteProfile(4);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/session");
    expect(fetchMock.mock.calls[1][1].headers["X-DumperSG-Token"]).toBe(
      "session-secret",
    );
    expect(fetchMock.mock.calls[2][1].headers["X-DumperSG-Token"]).toBe(
      "session-secret",
    );
    expect(fetchMock.mock.calls[1][0]).not.toContain("secret");
  });
  it("faz leitura de eventos incremental sem buscar sessão nem token de mutação", async () => {
    fetchMock.mockResolvedValue(response([]));
    const { api } = await import("./api");
    await api.events("job/id", 41);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/jobs/job%2Fid/events?after=41",
      expect.objectContaining({ method: "GET" }),
    );
    expect(
      fetchMock.mock.calls[0][1].headers["X-DumperSG-Token"],
    ).toBeUndefined();
  });
  it("preserva a semântica de senha omitida ao editar", async () => {
    fetchMock
      .mockResolvedValueOnce(response({ token: "token" }))
      .mockResolvedValueOnce(response({ id: 1 }));
    const { api } = await import("./api");
    await api.updateProfile(1, { name: "Outro nome" });
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({
      name: "Outro nome",
    });
  });
  it("propaga mensagem e status de conflitos operacionais", async () => {
    fetchMock.mockImplementation(() =>
      Promise.resolve(
        response({ error: "Já existe uma operação ativa." }, 409),
      ),
    );
    const { api, APIError } = await import("./api");
    await expect(api.jobs()).rejects.toBeInstanceOf(APIError);
    await expect(api.jobs()).rejects.toMatchObject({
      status: 409,
      message: "Já existe uma operação ativa.",
    });
  });
  it("permite nova tentativa de sessão quando a primeira falha", async () => {
    fetchMock
      .mockRejectedValueOnce(new TypeError("network"))
      .mockResolvedValueOnce(response({ token: "retry-token" }))
      .mockResolvedValueOnce(response(null, 204));
    const { api } = await import("./api");
    await expect(api.deleteProfile(1)).rejects.toThrow("network");
    await api.deleteProfile(1);
    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/session");
    expect(fetchMock.mock.calls[2][1].headers["X-DumperSG-Token"]).toBe(
      "retry-token",
    );
  });
});
