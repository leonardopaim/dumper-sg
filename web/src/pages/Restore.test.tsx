import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Restore } from "./Restore";
import { api } from "../api";
import type { Profile, Job } from "../types";
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
const job: Job = {
  id: "job1",
  kind: "restore",
  profile_id: 1,
  profile_name: "Local",
  database: "restaurado",
  path: "/backup",
  status: "running",
  progress: 0,
  message: "",
  started_at: "2026-10-07T12:00:00Z",
};
const props = {
  backups: [],
  reloadBackups: vi.fn().mockResolvedValue(undefined),
  onJob: vi.fn(),
  active: false,
  onProfiles: vi.fn(),
};
describe("restauração", () => {
  it("impede banco padrão e destino remoto", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<Restore {...props} profiles={[profile]} />);
    await user.type(
      screen.getByLabelText(/Banco isolado de destino/),
      "origem",
    );
    expect(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Criar banco" })).toBeDisabled();
    rerender(
      <Restore
        {...props}
        profiles={[{ ...profile, host: "remote.example" }]}
      />,
    );
    await user.clear(screen.getByLabelText(/Banco isolado de destino/));
    await user.type(
      screen.getByLabelText(/Banco isolado de destino/),
      "restaurado",
    );
    expect(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    ).toBeDisabled();
  });
  it("permite o alias local do Docker Desktop", async () => {
    const user = userEvent.setup();
    render(
      <Restore
        {...props}
        profiles={[{ ...profile, host: "host.docker.internal" }]}
      />,
    );
    await user.type(
      screen.getByLabelText(/Banco isolado de destino/),
      "restaurado",
    );
    expect(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    ).toBeEnabled();
    expect(screen.getByRole("button", { name: "Criar banco" })).toBeEnabled();
  });
  it("envia overwrite somente após confirmar o nome exato do destino", async () => {
    const restore = vi.spyOn(api, "restore").mockResolvedValue(job);
    const user = userEvent.setup();
    render(<Restore {...props} profiles={[profile]} />);
    await user.type(screen.getByLabelText(/Diretório do backup/), "/backup");
    await user.type(
      screen.getByLabelText(/Banco isolado de destino/),
      "restaurado",
    );
    await user.click(
      screen.getByRole("checkbox", { name: /Sobrescrever tabelas/ }),
    );
    await user.click(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    );
    expect(restore).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "Sobrescrever e restaurar" }),
    ).toBeDisabled();
    await user.type(
      screen.getByLabelText("Digite o banco de destino para confirmar"),
      "restaurado",
    );
    await user.click(
      screen.getByRole("button", { name: "Sobrescrever e restaurar" }),
    );
    expect(restore).toHaveBeenCalledWith(
      expect.objectContaining({
        overwrite_tables: true,
        target_database: "restaurado",
        backup_dir: "/backup",
      }),
    );
    restore.mockRestore();
  });
});
