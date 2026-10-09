import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Backup } from "./Backup";
import { backupDraftKey } from "../hooks/useOperationDraft";
import { api } from "../api";
import type { Profile, Job } from "../types";

const profile: Profile = {
  id: 1,
  name: "Origem",
  host: "localhost",
  port: 3306,
  user: "qa",
  database: "origem",
  threads: 2,
  ssl: false,
  has_password: true,
};
const props = {
  settings: {},
  active: false,
  onJob: vi.fn(),
  onProfiles: vi.fn(),
};
beforeEach(() => localStorage.clear());
describe("preferências do backup", () => {
  it("lembra a seleção e preserva os campos quando os perfis são reordenados", async () => {
    const second = {
      ...profile,
      id: 2,
      name: "Outra origem",
      database: "outro",
      ssl: true,
    };
    const user = userEvent.setup();
    const view = render(<Backup {...props} profiles={[profile, second]} />);
    await user.selectOptions(screen.getByLabelText("Perfil de conexão"), "2");
    await user.clear(screen.getByLabelText("Banco de origem"));
    await user.type(screen.getByLabelText("Banco de origem"), "personalizado");
    await user.type(
      screen.getByLabelText(/Diretório de destino/),
      "C:\\backup",
    );
    expect(JSON.parse(localStorage.getItem(backupDraftKey)!)).toMatchObject({
      profile_id: 2,
      database: "personalizado",
      destination_dir: "C:\\backup",
      ssl: true,
    });
    view.unmount();
    render(<Backup {...props} profiles={[second, profile]} />);
    expect(screen.getByLabelText("Perfil de conexão")).toHaveValue("2");
    expect(screen.getByLabelText("Banco de origem")).toHaveValue(
      "personalizado",
    );
    expect(screen.getByLabelText(/Diretório de destino/)).toHaveValue(
      "C:\\backup",
    );
  });
  it("usa os padrões com dados inválidos e não copia campos de credenciais", () => {
    localStorage.setItem(
      backupDraftKey,
      JSON.stringify({
        profile_id: "2",
        database: 42,
        password: "secret",
        user: "other",
      }),
    );
    render(<Backup {...props} profiles={[profile]} />);
    expect(screen.getByLabelText("Perfil de conexão")).toHaveValue("1");
    expect(screen.getByLabelText("Banco de origem")).toHaveValue("origem");
    expect(localStorage.getItem(backupDraftKey)).not.toMatch(
      /password|secret|user/,
    );
  });
});

afterEach(() => vi.restoreAllMocks());
describe("limite de threads no host de produção", () => {
  const production: Profile = {
    ...profile,
    host: " DB.SOMMUSGESTOR.COM. ",
    threads: 8,
  };
  const job: Job = {
    id: "safe",
    kind: "backup",
    status: "running",
    profile_id: 1,
    profile_name: "Produção",
    database: "origem",
    path: "/backup",
    started_at: "2026-10-09T12:00:00Z",
    progress: 0,
    message: "Iniciado",
  };
  it("bloqueia o padrão herdado acima de 2, explica as conexões e aceita a correção", async () => {
    const start = vi.spyOn(api, "backup").mockResolvedValue(job);
    const onJob = vi.fn();
    const user = userEvent.setup();
    render(<Backup {...props} profiles={[production]} onJob={onJob} />);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Cada thread aumenta o número de conexões no banco de dados de produção",
    );
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeDisabled();
    expect(start).not.toHaveBeenCalled();
    const threads = screen.getByRole("spinbutton", { name: /Threads/ });
    await user.clear(threads);
    await user.type(threads, "2");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Iniciar backup" }));
    expect(start).toHaveBeenCalledWith(expect.objectContaining({ threads: 2 }));
    expect(onJob).toHaveBeenCalledWith(job);
  });
  it("bloqueia uma preferência explícita antiga e revalida ao trocar de perfil", async () => {
    localStorage.setItem(backupDraftKey, JSON.stringify({ threads: 3 }));
    const user = userEvent.setup();
    render(
      <Backup
        {...props}
        profiles={[
          { ...production, threads: 2 },
          { ...profile, id: 2 },
        ]}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeDisabled();
    await user.selectOptions(screen.getByLabelText("Perfil de conexão"), "2");
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Perfil de conexão"), "1");
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeDisabled();
  });
});
