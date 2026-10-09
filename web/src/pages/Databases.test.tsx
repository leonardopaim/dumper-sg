import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Databases } from "./Databases";
import { api } from "../api";
import type { DatabaseCatalog, Profile } from "../types";

const profile: Profile = {
  id: 1,
  name: "Origem",
  host: "localhost",
  port: 3306,
  user: "qa",
  database: "",
  ssl: false,
  threads: 2,
  has_password: true,
};
const catalog: DatabaseCatalog = {
  databases: [
    {
      name: "sommusgestor_12",
      group_id: 12,
      group_name: "Comércio São José",
      companies: [
        {
          company_id: 2,
          group_id: 12,
          legal_name: "Mercado José Ltda",
          trade_name: "Loja São José",
        },
      ],
    },
    { name: "outro" },
  ],
};
const props = {
  profiles: [profile],
  active: false,
  onJob: vi.fn(),
  onBackup: vi.fn(),
  onProfiles: vi.fn(),
};
afterEach(() => {
  vi.restoreAllMocks();
  props.onBackup.mockClear();
});

describe("bancos disponíveis", () => {
  it("distingue database e ID completos dos prefixos e permite escolher pesquisa parcial", async () => {
    vi.spyOn(api, "databases").mockResolvedValue({
      databases: [
        { name: "sommusgestor_1", group_id: 1, group_name: "Grupo Um" },
        { name: "sommusgestor_10", group_id: 10, group_name: "Grupo Dez" },
        { name: "sommusgestor_100", group_id: 100, group_name: "Grupo Cem" },
      ],
    });
    const user = userEvent.setup();
    render(<Databases {...props} />);
    await user.click(screen.getByRole("button", { name: "Carregar bancos" }));
    await screen.findByText("Grupo Um");
    const search = screen.getByRole("searchbox");
    const mode = screen.getByLabelText("Tipo de pesquisa");
    await user.type(search, " SOMMUSGESTOR_1 ");
    expect(screen.getByText("Grupo Um")).toBeInTheDocument();
    expect(screen.queryByText("Grupo Dez")).not.toBeInTheDocument();
    expect(screen.queryByText("Grupo Cem")).not.toBeInTheDocument();
    await user.selectOptions(mode, "contains");
    expect(screen.getByText("Grupo Dez")).toBeInTheDocument();
    expect(screen.getByText("Grupo Cem")).toBeInTheDocument();
    await user.selectOptions(mode, "exact");
    expect(screen.queryByText("Grupo Dez")).not.toBeInTheDocument();
    await user.selectOptions(mode, "auto");
    await user.clear(search);
    await user.type(search, "1");
    expect(screen.getByText("Grupo Um")).toBeInTheDocument();
    expect(screen.queryByText("Grupo Dez")).not.toBeInTheDocument();
    await user.clear(search);
    expect(screen.getByText("Grupo Dez")).toBeInTheDocument();
  });

  it("busca razão social e nome fantasia e prepara o banco inteiro do grupo", async () => {
    vi.spyOn(api, "databases").mockResolvedValue(catalog);
    const user = userEvent.setup();
    render(<Databases {...props} />);
    await user.click(screen.getByRole("button", { name: "Carregar bancos" }));
    await screen.findByText("Mercado José Ltda");
    const search = screen.getByRole("searchbox");
    for (const value of ["mercado jose", "jose loja"]) {
      await user.clear(search);
      await user.type(search, value);
      expect(screen.getByText("Comércio São José")).toBeInTheDocument();
      expect(screen.queryByText("outro")).not.toBeInTheDocument();
    }
    await user.selectOptions(
      screen.getByLabelText("Tipo de pesquisa"),
      "exact",
    );
    expect(screen.queryByText("Comércio São José")).not.toBeInTheDocument();
    for (const value of [
      "LOJA SAO JOSE",
      "comercio sao jose",
      "mercado jose ltda",
    ]) {
      await user.clear(search);
      await user.type(search, value);
      expect(screen.getByText("Comércio São José")).toBeInTheDocument();
    }
    await user.click(
      screen.getByRole("button", {
        name: "Preparar backup de Comércio São José",
      }),
    );
    expect(props.onBackup).toHaveBeenCalledWith({
      profile_id: 1,
      database: "sommusgestor_12",
    });
  });
  it("pesquisa grupos sem acentos, ID e database e prepara a origem selecionada", async () => {
    const load = vi.spyOn(api, "databases").mockResolvedValue(catalog);
    const backup = vi.spyOn(api, "backup");
    const user = userEvent.setup();
    render(<Databases {...props} />);
    await user.click(screen.getByRole("button", { name: "Carregar bancos" }));
    await screen.findByText("Comércio São José");
    expect(load).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    const search = screen.getByRole("searchbox");
    for (const value of ["comercio sao", "12", "sommusgestor_12"]) {
      await user.clear(search);
      await user.type(search, value);
      expect(screen.getByText("Comércio São José")).toBeInTheDocument();
      expect(screen.queryByText("outro")).not.toBeInTheDocument();
    }
    await user.click(
      screen.getByRole("button", {
        name: "Preparar backup de Comércio São José",
      }),
    );
    expect(props.onBackup).toHaveBeenCalledWith({
      profile_id: 1,
      database: "sommusgestor_12",
    });
    expect(backup).not.toHaveBeenCalled();
  });

  it("descarta a consulta antiga quando o perfil muda", async () => {
    let finish!: (value: DatabaseCatalog) => void;
    let signal!: AbortSignal;
    vi.spyOn(api, "databases").mockImplementation((_id, options) => {
      signal = options!.signal!;
      return new Promise((resolve) => {
        finish = resolve;
      });
    });
    const user = userEvent.setup();
    render(
      <Databases
        {...props}
        profiles={[profile, { ...profile, id: 2, name: "Outro servidor" }]}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Carregar bancos" }));
    await user.selectOptions(screen.getByLabelText("Perfil de conexão"), "2");
    expect(signal.aborted).toBe(true);
    finish(catalog);
    await waitFor(() =>
      expect(screen.queryByText("Comércio São José")).not.toBeInTheDocument(),
    );
    expect(
      screen.getByRole("button", { name: "Carregar bancos" }),
    ).toBeEnabled();
  });

  it("mostra o aviso e permite selecionar um database sem nome do grupo", async () => {
    vi.spyOn(api, "databases").mockResolvedValue({
      databases: [{ name: "sommusgestor_12" }],
      warning: "Nomes dos grupos indisponíveis.",
    });
    const user = userEvent.setup();
    render(<Databases {...props} />);
    await user.click(screen.getByRole("button", { name: "Carregar bancos" }));
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Nomes dos grupos indisponíveis.",
    );
    await user.click(
      screen.getByRole("button", {
        name: "Preparar backup de sommusgestor_12",
      }),
    );
    expect(props.onBackup).toHaveBeenCalledWith({
      profile_id: 1,
      database: "sommusgestor_12",
    });
  });

  it("exibe falhas e bloqueia novas consultas durante uma operação", async () => {
    vi.spyOn(api, "databases").mockRejectedValue(new Error("Conexão recusada"));
    const user = userEvent.setup();
    const view = render(<Databases {...props} />);
    await user.click(screen.getByRole("button", { name: "Carregar bancos" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Conexão recusada",
    );
    view.rerender(<Databases {...props} active />);
    expect(
      screen.getByRole("button", { name: "Carregar bancos" }),
    ).toBeDisabled();
  });
});
