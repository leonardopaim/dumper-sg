import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Backups } from "./Backups";
import { api } from "../api";
import type { BackupEntry } from "../types";

const backup: BackupEntry = {
  id: "a".repeat(64),
  path: "C:\\backups\\origem",
  name: "origem",
  modified_at: "2026-10-08T12:00:00Z",
  size_bytes: 1024,
  file_count: 3,
  complete: true,
};
const props = {
  backups: [backup],
  reload: vi.fn().mockResolvedValue(undefined),
  onRestore: vi.fn(),
  active: false,
};
beforeEach(() => {
  vi.restoreAllMocks();
  props.reload.mockClear();
  props.onRestore.mockClear();
});

describe("gestão dos backups", () => {
  it("mostra arquivos e tabelas, abre a pasta e encaminha a origem para restauração", async () => {
    const user = userEvent.setup();
    const open = vi.spyOn(api, "openBackup").mockResolvedValue(undefined);
    vi.spyOn(api, "backupTables").mockResolvedValue([
      {
        database: "db",
        name: "clientes",
        size_bytes: 128,
        rows: 3,
        table_type: "BASE TABLE",
      },
    ]);
    render(<Backups {...props} />);
    await user.click(
      screen.getByRole("button", { name: "Abrir pasta de origem" }),
    );
    expect(open).toHaveBeenCalledWith(backup.id);
    await user.click(
      screen.getByRole("button", { name: "Ver detalhes de origem" }),
    );
    const dialog = screen.getByRole("dialog", { name: "Detalhes do backup" });
    expect(await within(dialog).findByText("db.clientes")).toBeInTheDocument();
    expect(within(dialog).getByText(backup.path)).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole("button", { name: "Restaurar este backup" }),
    );
    expect(props.onRestore).toHaveBeenCalledWith(backup.path);
  });
  it("exige confirmação, permite cancelar e atualiza o catálogo somente após excluir", async () => {
    const user = userEvent.setup();
    const remove = vi.spyOn(api, "deleteBackup").mockResolvedValue(undefined);
    render(<Backups {...props} />);
    await user.click(screen.getByRole("button", { name: "Excluir origem" }));
    expect(remove).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(remove).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Excluir origem" }));
    await user.click(
      screen.getByRole("button", { name: "Excluir permanentemente" }),
    );
    expect(remove).toHaveBeenCalledWith(backup.id);
    expect(props.reload).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Backup excluído");
  });
  it("mantém a confirmação aberta e mostra a causa quando a API bloqueia a exclusão", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "deleteBackup").mockRejectedValue(
      new Error("Container com término pendente"),
    );
    render(<Backups {...props} />);
    await user.click(screen.getByRole("button", { name: "Excluir origem" }));
    await user.click(
      screen.getByRole("button", { name: "Excluir permanentemente" }),
    );
    expect(
      within(screen.getByRole("dialog")).getByRole("alert"),
    ).toHaveTextContent("Container com término pendente");
    expect(props.reload).not.toHaveBeenCalled();
  });
  it("bloqueia exclusão durante operações e impede restaurar backups incompletos", async () => {
    const user = userEvent.setup();
    const query = vi.spyOn(api, "backupTables");
    const partial = { ...backup, complete: false };
    render(<Backups {...props} backups={[partial]} active />);
    expect(
      screen.getByRole("button", { name: "Excluir origem" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Restaurar origem" }),
    ).toBeDisabled();
    await user.click(
      screen.getByRole("button", { name: "Ver detalhes de origem" }),
    );
    expect(
      within(screen.getByRole("dialog")).getByRole("alert"),
    ).toHaveTextContent("marca de conclusão");
    expect(query).not.toHaveBeenCalled();
  });
  it("filtra pelo caminho e ordena por tamanho", async () => {
    const user = userEvent.setup();
    render(
      <Backups
        {...props}
        backups={[
          backup,
          {
            ...backup,
            id: "b".repeat(64),
            name: "maior",
            path: "D:\\custom\\maior",
            size_bytes: 2048,
            modified_at: "2026-10-07T12:00:00Z",
          },
        ]}
      />,
    );
    await user.selectOptions(screen.getByLabelText("Ordenar backups"), "size");
    expect(screen.getAllByRole("row")[1]).toHaveTextContent("maior");
    await user.type(screen.getByLabelText("Buscar backup"), "D:\\custom");
    expect(
      screen.getByRole("button", { name: "Ver detalhes de maior" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Ver detalhes de origem" }),
    ).not.toBeInTheDocument();
    await user.clear(screen.getByLabelText("Buscar backup"));
    await user.type(screen.getByLabelText("Buscar backup"), "não existe");
    await waitFor(() =>
      expect(
        screen.getByText("Nenhum backup corresponde à busca"),
      ).toBeInTheDocument(),
    );
  });
});
