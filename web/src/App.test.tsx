import { describe, expect, it, vi, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "./App";
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
});
describe("navegação durante operações", () => {
  it("mantém o monitor e permite cancelar mesmo depois de alternar de tela", async () => {
    location.hash = "#dashboard";
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
    await waitFor(() =>
      expect(screen.getByText("Tabela real do processo")).toBeInTheDocument(),
    );
    await user.click(screen.getByRole("link", { name: /Perfis de banco/ }));
    expect(
      screen.getByRole("heading", { name: "Perfis de banco" }),
    ).toBeInTheDocument();
    const monitor = screen.getByRole("region", {
      name: "Acompanhamento da operação",
    });
    expect(
      within(monitor).getByText("Tabela real do processo"),
    ).toBeInTheDocument();
    await user.click(within(monitor).getByRole("button", { name: "Cancelar" }));
    expect(cancel).toHaveBeenCalledWith("job1");
    expect(
      within(monitor).getByRole("button", { name: "Cancelar" }),
    ).toBeDisabled();
  });
});
