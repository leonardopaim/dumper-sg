import { describe, expect, it, vi, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App, monitorPreferenceKey } from "./App";
import { api } from "./api";
import type { Job, Profile } from "./types";
const profile: Profile = {
  id: 1,
  name: "Local",
  host: "127.0.0.1",
  port: 3306,
  user: "root",
  database: "origem",
  ssl: false,
  threads: 8,
  has_password: true,
};
const running: Job = {
  id: "job1",
  kind: "backup",
  status: "running",
  profile_id: 1,
  profile_name: "Local",
  database: "origem",
  path: "/backup",
  started_at: "2026-10-07T12:00:00Z",
  progress: 32,
  message: "Exportando",
};
afterEach(() => {
  vi.restoreAllMocks();
  location.hash = "";
  localStorage.clear();
});
function mockIdleAPI() {
  vi.spyOn(api, "profiles").mockResolvedValue([profile]);
  vi.spyOn(api, "settings").mockResolvedValue({
    default_backup_dir: "/backup",
  });
  vi.spyOn(api, "backups").mockResolvedValue([]);
  vi.spyOn(api, "history").mockResolvedValue([]);
  vi.spyOn(api, "diagnostics").mockResolvedValue({
    available: true,
    version: "Docker test",
    message: "Pronto",
  });
  vi.spyOn(api, "health").mockResolvedValue({ status: "ok", version: "test" });
  vi.spyOn(api, "jobs").mockResolvedValue([]);
  vi.spyOn(api, "job").mockResolvedValue(running);
  vi.spyOn(api, "events").mockResolvedValue([]);
}
describe("navegação durante operações", () => {
  it("permite reiniciar o backend com uma pendência histórica de container", async () => {
    mockIdleAPI();
    vi.spyOn(api, "application").mockResolvedValue({
      instance_id: "current",
      restart_available: true,
    });
    const pending: Job = {
      ...running,
      status: "failed",
      cleanup_required: true,
    };
    vi.spyOn(api, "jobs").mockResolvedValue([pending]);
    vi.spyOn(api, "job").mockResolvedValue(pending);
    const user = userEvent.setup();
    render(<App />);
    await screen.findByText("Core local conectado");
    const button = screen.getByRole("button", { name: "Reiniciar aplicação" });
    expect(button).toBeEnabled();
    await user.click(button);
    expect(
      screen.getByRole("button", { name: "Confirmar reinício" }),
    ).toBeEnabled();
  });
  it("oferece o controle antes de qualquer operação e não reabre o painel ao iniciar um backup", async () => {
    location.hash = "#backup";
    mockIdleAPI();
    vi.spyOn(api, "backup").mockResolvedValue(running);
    const user = userEvent.setup();
    let view = render(<App />);
    const submit = await screen.findByRole("button", {
      name: "Iniciar backup",
    });
    await user.click(screen.getByRole("button", { name: "Mostrar painel" }));
    expect(
      screen.getByText("Nenhuma operação selecionada"),
    ).toBeInTheDocument();
    expect(localStorage.getItem(monitorPreferenceKey)).toBe("true");
    await user.click(screen.getByRole("button", { name: "Ocultar painel" }));
    await user.click(submit);
    await waitFor(() => expect(api.events).toHaveBeenCalled());
    expect(
      screen.queryByRole("region", { name: "Acompanhamento da operação" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Mostrar painel" }));
    expect(screen.getByText("32% estimado")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Ocultar painel" }));
    view.unmount();
    view = render(<App />);
    expect(
      screen.getByRole("button", { name: "Mostrar painel" }),
    ).toHaveAttribute("aria-expanded", "false");
    expect(
      screen.queryByRole("region", { name: "Acompanhamento da operação" }),
    ).not.toBeInTheDocument();
  });
  it("restaura a escolha de exibir o painel após recarregar", async () => {
    mockIdleAPI();
    localStorage.setItem(monitorPreferenceKey, "true");
    render(<App />);
    expect(
      screen.getByRole("button", { name: "Ocultar painel" }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByRole("region", { name: "Acompanhamento da operação" }),
    ).toBeInTheDocument();
    await screen.findByRole("heading", {
      name: "Seus dados, protegidos.",
    });
  });
  it("mantém o monitor e permite cancelar mesmo depois de alternar de tela", async () => {
    location.hash = "#dashboard";
    localStorage.clear();
    vi.spyOn(api, "profiles").mockResolvedValue([profile]);
    vi.spyOn(api, "settings").mockResolvedValue({
      default_backup_dir: "/backup",
    });
    vi.spyOn(api, "backups").mockResolvedValue([]);
    vi.spyOn(api, "history").mockResolvedValue([running]);
    vi.spyOn(api, "diagnostics").mockResolvedValue({
      available: true,
      version: "Docker test",
      message: "Pronto",
    });
    vi.spyOn(api, "health").mockResolvedValue({
      status: "ok",
      version: "test",
    });
    vi.spyOn(api, "jobs").mockResolvedValue([running]);
    vi.spyOn(api, "job").mockResolvedValue(running);
    vi.spyOn(api, "events").mockResolvedValue([
      {
        sequence: 1,
        time: running.started_at,
        level: "info",
        message: "Tabela real do processo",
      },
    ]);
    const cancel = vi
      .spyOn(api, "cancel")
      .mockResolvedValue({ ...running, status: "cancel_requested" });
    const user = userEvent.setup();
    render(<App />);
    await waitFor(() => expect(api.events).toHaveBeenCalled());
    expect(
      screen.queryByText("Tabela real do processo"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("region", { name: "Acompanhamento da operação" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Mostrar painel" }));
    await user.click(screen.getByRole("button", { name: "Mostrar logs" }));
    await waitFor(() =>
      expect(screen.getByText("Tabela real do processo")).toBeInTheDocument(),
    );
    await user.click(screen.getByRole("button", { name: "Ocultar logs" }));
    await user.click(screen.getByRole("link", { name: /Perfis de banco/ }));
    expect(
      screen.getByRole("heading", { name: "Perfis de banco" }),
    ).toBeInTheDocument();
    const monitor = screen.getByRole("region", {
      name: "Acompanhamento da operação",
    });
    expect(
      within(monitor).queryByText("Tabela real do processo"),
    ).not.toBeInTheDocument();
    expect(within(monitor).getByText("32% estimado")).toBeInTheDocument();
    expect(
      monitor.compareDocumentPosition(
        screen.getByRole("heading", { name: "Perfis de banco" }),
      ) & Node.DOCUMENT_POSITION_PRECEDING,
    ).toBeTruthy();
    await user.click(within(monitor).getByRole("button", { name: "Cancelar" }));
    expect(cancel).toHaveBeenCalledWith("job1");
    expect(
      within(monitor).getByRole("button", { name: "Cancelar" }),
    ).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Ocultar painel" }));
    expect(
      screen.queryByRole("region", { name: "Acompanhamento da operação" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Mostrar painel" }),
    ).toBeEnabled();
    expect(localStorage.getItem(monitorPreferenceKey)).toBe("false");
  });
});
