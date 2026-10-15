import { afterEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Environment } from "./Environment";
import { api } from "../api";
import type { EnvironmentReport } from "../types";
const snapshot: EnvironmentReport = {
  checked_at: "2026-10-09T12:00:00Z",
  platform: "windows",
  architecture: "amd64",
  go_version: "go1.26",
  api_address: "127.0.0.1:8787",
  database_path: "C:\\data\\dumper_sg.sqlite3",
  directories: [
    {
      name: "Backups",
      path: "D:\\backups",
      exists: true,
      readable: true,
      writable: true,
    },
  ],
  docker: {
    available: true,
    runtime: "wsl",
    distribution: "Ubuntu",
    discovery_pending: false,
    engine: "Docker Engine",
    version: "28.2",
    daemon_name: "qa-linux",
    daemon_id: "selected",
    container_os: "linux",
    network: "Rede host",
    backup_mount: "/mnt/d/backups:/backup",
    backup_mount_accessible: true,
    message: "Disponível via WSL",
    images: [
      {
        name: "dumper:test",
        present: false,
        message: "Baixar no primeiro uso",
      },
    ],
  },
};
afterEach(() => vi.restoreAllMocks());
it("consulta o ambiente efetivo e permite atualizar a mesma tela", async () => {
  const query = vi
    .spyOn(api, "environment")
    .mockResolvedValueOnce(snapshot)
    .mockResolvedValue({
      ...snapshot,
      docker: {
        ...snapshot.docker,
        available: false,
        message: "Docker não respondeu",
      },
    });
  const user = userEvent.setup();
  render(<Environment onBack={vi.fn()} />);
  expect(await screen.findByText("WSL · Ubuntu")).toBeInTheDocument();
  expect(screen.getByText("D:\\backups")).toBeInTheDocument();
  expect(screen.getByText("/mnt/d/backups:/backup")).toBeInTheDocument();
  expect(screen.getByText("Verificar no primeiro uso")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Atualizar ambiente" }));
  expect(await screen.findByText("Docker não respondeu")).toBeInTheDocument();
  expect(query).toHaveBeenCalledTimes(2);
});
it("mostra erro de consulta sem inventar disponibilidade", async () => {
  vi.spyOn(api, "environment").mockRejectedValue(
    new Error("Falha ao consultar o ambiente"),
  );
  render(<Environment onBack={vi.fn()} />);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Falha ao consultar",
  );
  expect(screen.queryByText("Disponível")).not.toBeInTheDocument();
});
