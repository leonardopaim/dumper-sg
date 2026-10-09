import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Restore } from "./Restore";
import { api } from "../api";
import { restoreDraftKey } from "../hooks/useOperationDraft";
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
  beforeEach(() => localStorage.clear());
  it("recebe a origem da gestão de backups sem substituir o destino e persiste a escolha", () => {
    localStorage.setItem(
      restoreDraftKey,
      JSON.stringify({
        profile_id: 1,
        backup_dir: "anterior",
        target_database: "meu_destino",
        threads: 4,
        overwrite_tables: true,
      }),
    );
    const applied = vi.fn();
    const view = render(
      <Restore
        {...props}
        profiles={[profile]}
        initialBackup={"C:\\backup_escolhido"}
        onBackupApplied={applied}
      />,
    );
    expect(screen.getByLabelText(/Diretório do backup/)).toHaveValue(
      "C:\\backup_escolhido",
    );
    expect(screen.getByLabelText(/Banco isolado de destino/)).toHaveValue(
      "meu_destino",
    );
    expect(
      screen.getByRole("checkbox", { name: /Sobrescrever tabelas/ }),
    ).not.toBeChecked();
    expect(applied).toHaveBeenCalledOnce();
    view.unmount();
    render(<Restore {...props} profiles={[profile]} />);
    expect(screen.getByLabelText(/Diretório do backup/)).toHaveValue(
      "C:\\backup_escolhido",
    );
  });
  it("recupera o perfil e os campos ao voltar à tela, mantendo sobrescrita desmarcada", async () => {
    const profiles = [profile, { ...profile, id: 2, name: "Outro local" }];
    const user = userEvent.setup();
    const view = render(<Restore {...props} profiles={profiles} />);
    await user.selectOptions(screen.getByLabelText("Perfil local"), "2");
    await user.type(screen.getByLabelText(/Diretório do backup/), "C:\\backup");
    await user.type(
      screen.getByLabelText(/Banco isolado de destino/),
      "destino",
    );
    await user.clear(screen.getByLabelText(/Threads/));
    await user.type(screen.getByLabelText(/Threads/), "4");
    await user.click(
      screen.getByRole("checkbox", { name: /Sobrescrever tabelas/ }),
    );
    const saved = JSON.parse(localStorage.getItem(restoreDraftKey)!);
    expect(saved).toEqual({
      profile_id: 2,
      backup_dir: "C:\\backup",
      target_database: "destino",
      threads: 4,
    });
    view.unmount();
    render(<Restore {...props} profiles={profiles} />);
    expect(screen.getByLabelText("Perfil local")).toHaveValue("2");
    expect(screen.getByLabelText(/Diretório do backup/)).toHaveValue(
      "C:\\backup",
    );
    expect(screen.getByLabelText(/Banco isolado de destino/)).toHaveValue(
      "destino",
    );
    expect(screen.getByLabelText(/Threads/)).toHaveValue(4);
    expect(
      screen.getByRole("checkbox", { name: /Sobrescrever tabelas/ }),
    ).not.toBeChecked();
  });
  it("aguarda os perfis e descarta a seleção somente se o perfil foi removido", () => {
    localStorage.setItem(
      restoreDraftKey,
      JSON.stringify({ profile_id: 2, target_database: "destino" }),
    );
    const view = render(<Restore {...props} profiles={[]} />);
    view.rerender(
      <Restore {...props} profiles={[profile, { ...profile, id: 2 }]} />,
    );
    expect(screen.getByLabelText("Perfil local")).toHaveValue("2");
    view.rerender(<Restore {...props} profiles={[profile]} />);
    expect(screen.getByLabelText("Perfil local")).toHaveValue("1");
    expect(screen.getByLabelText(/Banco isolado de destino/)).toHaveValue("");
  });
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
  it.each([false, true])(
    "confirma o resumo sem digitar o banco, com sobrescrita=%s",
    async (overwrite) => {
      const restore = vi.spyOn(api, "restore").mockResolvedValue(job);
      const user = userEvent.setup();
      render(<Restore {...props} profiles={[profile]} />);
      await user.type(screen.getByLabelText(/Diretório do backup/), "/backup");
      await user.type(
        screen.getByLabelText(/Banco isolado de destino/),
        "restaurado",
      );
      if (overwrite)
        await user.click(
          screen.getByRole("checkbox", { name: /Sobrescrever tabelas/ }),
        );
      await user.click(
        screen.getByRole("button", { name: "Iniciar restauração" }),
      );
      expect(restore).not.toHaveBeenCalled();
      let modal = screen.getByRole("dialog", { name: "Confirmar restauração" });
      expect(within(modal).getByText("Local")).toBeInTheDocument();
      expect(within(modal).getByText("127.0.0.1:3306")).toBeInTheDocument();
      expect(within(modal).getByText("restaurado")).toBeInTheDocument();
      expect(within(modal).getByText("/backup")).toBeInTheDocument();
      expect(within(modal).getByText("8")).toBeInTheDocument();
      expect(
        within(modal).getByText(overwrite ? "Sim" : "Não"),
      ).toBeInTheDocument();
      expect(within(modal).queryByRole("textbox")).not.toBeInTheDocument();
      expect(
        within(modal).getByRole("button", { name: "Confirmar restauração" }),
      ).toHaveClass(overwrite ? "danger" : "primary");
      await user.click(within(modal).getByRole("button", { name: "Cancelar" }));
      expect(restore).not.toHaveBeenCalled();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      await user.click(
        screen.getByRole("button", { name: "Iniciar restauração" }),
      );
      modal = screen.getByRole("dialog", { name: "Confirmar restauração" });
      await user.click(
        within(modal).getByRole("button", { name: "Confirmar restauração" }),
      );
      expect(restore).toHaveBeenCalledWith(
        expect.objectContaining({
          overwrite_tables: overwrite,
          target_database: "restaurado",
          backup_dir: "/backup",
          threads: 0,
        }),
      );
      restore.mockRestore();
    },
  );
});
