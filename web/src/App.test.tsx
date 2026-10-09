import { describe, expect, it, vi, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "./App";
import { api } from "./api";
import { themePreferenceKey } from "./hooks/useTheme";
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
  delete document.documentElement.dataset.theme;
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
  it("permite reiniciar com pendência histórica pela área de manutenção", async () => {
    location.hash = "#settings";
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
    await screen.findByRole("heading", { name: "Configurações" });
    await user.click(screen.getByText("Manutenção"));
    const button = await screen.findByRole("button", {
      name: "Reiniciar aplicação",
    });
    expect(button).toBeEnabled();
    await user.click(button);
    expect(
      screen.getByRole("button", { name: "Confirmar reinício" }),
    ).toBeEnabled();
  });
  it("abre o modal ao iniciar e permite minimizar e reabrir a operação", async () => {
    location.hash = "#backup";
    mockIdleAPI();
    vi.spyOn(api, "backup").mockResolvedValue(running);
    const user = userEvent.setup();
    render(<App />);
    await user.click(
      await screen.findByRole("button", { name: "Iniciar backup" }),
    );
    const modal = await screen.findByRole("dialog", {
      name: "Backup · Em andamento",
    });
    expect(within(modal).getByText("32% estimado")).toBeInTheDocument();
    await user.click(within(modal).getByRole("button", { name: "Minimizar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Ver operação" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
  it("recupera a operação ao recarregar sem interromper a navegação", async () => {
    mockIdleAPI();
    vi.spyOn(api, "jobs").mockResolvedValue([running]);
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole("button", { name: "Ver operação" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Ver operação" }));
    expect(screen.getByRole("dialog")).toHaveAccessibleName(
      "Backup · Em andamento",
    );
  });
  it("mantém progresso, logs e cancelamento ao navegar com o modal minimizado", async () => {
    mockIdleAPI();
    vi.spyOn(api, "jobs").mockResolvedValue([running]);
    vi.spyOn(api, "events").mockResolvedValue([
      {
        sequence: 1,
        time: running.started_at,
        level: "info",
        message: "Exportando tabelas",
      },
    ]);
    const cancel = vi
      .spyOn(api, "cancel")
      .mockResolvedValue({ ...running, status: "cancel_requested" });
    const user = userEvent.setup();
    render(<App />);
    await user.click(
      await screen.findByRole("button", { name: "Ver operação" }),
    );
    await user.click(screen.getByRole("button", { name: "Minimizar" }));
    await user.click(screen.getByRole("link", { name: /Perfis de banco/ }));
    expect(
      screen.getByRole("heading", { name: "Perfis de banco" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Ver operação" }));
    const monitor = within(screen.getByRole("dialog"));
    expect(monitor.getByText("32% estimado")).toBeInTheDocument();
    await user.click(monitor.getByRole("button", { name: "Mostrar logs" }));
    expect(await screen.findByText("Exportando tabelas")).toBeInTheDocument();
    await user.click(monitor.getByRole("button", { name: "Cancelar" }));
    expect(cancel).toHaveBeenCalledWith("job1");
    expect(monitor.getByRole("button", { name: "Cancelar" })).toBeDisabled();
  });
  it("mostra o resultado ao concluir e mantém o modal aberto até fechar", async () => {
    location.hash = "#backup";
    mockIdleAPI();
    let current: Job | undefined;
    vi.spyOn(api, "backup").mockImplementation(async () => {
      current = running;
      return running;
    });
    vi.spyOn(api, "jobs").mockImplementation(async () =>
      current ? [current] : [],
    );
    vi.spyOn(api, "job").mockImplementation(async () => current || running);
    const user = userEvent.setup();
    render(<App />);
    await user.click(
      await screen.findByRole("button", { name: "Iniciar backup" }),
    );
    await screen.findByRole("dialog", { name: "Backup · Em andamento" });
    current = {
      ...running,
      status: "succeeded",
      progress: 100,
      message: "Backup finalizado.",
    };
    await screen.findByRole(
      "heading",
      { name: "Operação concluída" },
      { timeout: 3500 },
    );
    expect(screen.getByRole("dialog")).toHaveAccessibleName(
      "Backup · Resultado",
    );
    await user.click(screen.getByRole("button", { name: "Concluir" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(api.history).toHaveBeenCalled();
  });
});

describe("tema da interface", () => {
  it("aplica a escolha imediatamente, mantém ao navegar e restaura ao reabrir", async () => {
    location.hash = "#settings";
    mockIdleAPI();
    const user = userEvent.setup();
    let view = render(<App />);
    const selector = await screen.findByRole("combobox", {
      name: /Tema da interface/,
    });
    expect(selector).toHaveValue("light");
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    await user.selectOptions(selector, "dark");
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(localStorage.getItem(themePreferenceKey)).toBe("dark");
    await user.click(screen.getByRole("link", { name: /Criar backup/ }));
    await screen.findByRole("button", { name: "Iniciar backup" });
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    view.unmount();
    location.hash = "#settings";
    view = render(<App />);
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    const restored = await screen.findByRole("combobox", {
      name: /Tema da interface/,
    });
    expect(restored).toHaveValue("dark");
    await user.selectOptions(restored, "light");
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(localStorage.getItem(themePreferenceKey)).toBe("light");
  });

  it("usa o tema claro quando a preferência salva é inválida", async () => {
    localStorage.setItem(themePreferenceKey, "invalid");
    mockIdleAPI();
    render(<App />);
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    await screen.findByText("Core local conectado");
  });

  it("permite mudar de tema quando o navegador bloqueia o armazenamento", async () => {
    location.hash = "#settings";
    mockIdleAPI();
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("Armazenamento bloqueado");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("Armazenamento bloqueado");
    });
    const user = userEvent.setup();
    render(<App />);
    const selector = await screen.findByRole("combobox", {
      name: /Tema da interface/,
    });
    expect(selector).toHaveValue("light");
    await user.selectOptions(selector, "dark");
    expect(selector).toHaveValue("dark");
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
  });
});

describe("seleção da origem pelo catálogo", () => {
  it("leva perfil e database para o formulário, preserva opções e não inicia backup", async () => {
    location.hash = "#databases";
    mockIdleAPI();
    localStorage.setItem(
      "dumpersg.backupDraft",
      JSON.stringify({
        profile_id: 999,
        database: "anterior",
        destination_dir: "C:\\backups",
        threads: 4,
      }),
    );
    vi.spyOn(api, "profiles").mockResolvedValue([
      profile,
      { ...profile, id: 2, name: "Outro servidor", ssl: true },
    ]);
    vi.spyOn(api, "databases").mockResolvedValue({
      databases: [
        { name: "sommusgestor_12", group_name: "São José", group_id: 12 },
      ],
    });
    const backup = vi.spyOn(api, "backup");
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole("heading", { name: "Bancos disponíveis" });
    await user.selectOptions(screen.getByLabelText("Perfil de conexão"), "2");
    await user.click(screen.getByRole("button", { name: "Carregar bancos" }));
    await user.click(
      await screen.findByRole("button", {
        name: "Preparar backup de São José",
      }),
    );
    expect(screen.getByLabelText("Perfil de conexão")).toHaveValue("2");
    expect(screen.getByLabelText("Banco de origem")).toHaveValue(
      "sommusgestor_12",
    );
    expect(screen.getByLabelText(/Diretório de destino/)).toHaveValue(
      "C:\\backups",
    );
    expect(screen.getByLabelText(/Threads/)).toHaveValue(4);
    expect(
      JSON.parse(localStorage.getItem("dumpersg.backupDraft")!),
    ).toMatchObject({ profile_id: 2, database: "sommusgestor_12", ssl: true });
    expect(backup).not.toHaveBeenCalled();
  });
});

describe("consultas no modal", () => {
  it("não reabre o modal minimizado quando uma consulta recebe seu resultado", async () => {
    location.hash = "#databases";
    mockIdleAPI();
    const query: Job = { ...running, kind: "database_list" };
    vi.spyOn(api, "job").mockResolvedValue(query);
    let finish!: () => void;
    vi.spyOn(api, "databases").mockImplementation(async (_id, options) => {
      options?.onJob?.(query);
      await new Promise<void>((resolve) => {
        finish = () => {
          options?.onJob?.({ ...query, status: "succeeded" });
          resolve();
        };
      });
      return { databases: [{ name: "banco_consultado" }] };
    });
    const user = userEvent.setup();
    render(<App />);
    await user.click(
      await screen.findByRole("button", { name: "Carregar bancos" }),
    );
    await screen.findByRole("dialog", {
      name: "Consulta de bancos · Em andamento",
    });
    await user.click(screen.getByRole("button", { name: "Minimizar" }));
    finish();
    await screen.findByText("banco_consultado");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ver operação" })).toBeEnabled();
  });
});

describe("preparação e falha antes de criar o job", () => {
  it("abre imediatamente e mostra o erro mesmo quando a API não cria uma operação", async () => {
    location.hash = "#backup";
    mockIdleAPI();
    let rejectStart!: (error: Error) => void;
    vi.spyOn(api, "backup").mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectStart = reject;
        }),
    );
    const user = userEvent.setup();
    render(<App />);
    await user.click(
      await screen.findByRole("button", { name: "Iniciar backup" }),
    );
    const modal = screen.getByRole("dialog", { name: "Backup · Preparando" });
    expect(modal).toHaveTextContent("Validando as configurações");
    rejectStart(new Error("Docker indisponível"));
    await screen.findByRole("heading", {
      name: "Não foi possível concluir a ação",
    });
    expect(
      within(screen.getByRole("dialog")).getByText("Docker indisponível"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Concluir" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("preserva a minimização quando a preparação termina e o job começa", async () => {
    location.hash = "#backup";
    mockIdleAPI();
    let finishStart!: (job: Job) => void;
    vi.spyOn(api, "backup").mockImplementation(
      () =>
        new Promise((resolve) => {
          finishStart = resolve;
        }),
    );
    const user = userEvent.setup();
    render(<App />);
    await user.click(
      await screen.findByRole("button", { name: "Iniciar backup" }),
    );
    await user.click(screen.getByRole("button", { name: "Minimizar" }));
    finishStart(running);
    await waitFor(() => expect(api.events).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Ver operação" }));
    expect(screen.getByRole("dialog")).toHaveAccessibleName(
      "Backup · Em andamento",
    );
  });
});

it("substitui a confirmação de restauração pela preparação e permite minimizar", async () => {
  location.hash = "#restore";
  mockIdleAPI();
  let finish!: (job: Job) => void;
  vi.spyOn(api, "restore").mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const user = userEvent.setup();
  render(<App />);
  await user.type(
    await screen.findByLabelText(/Diretório do backup/),
    "/backup",
  );
  await user.type(screen.getByLabelText(/Banco isolado de destino/), "isolado");
  await user.click(screen.getByRole("button", { name: "Iniciar restauração" }));
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", {
      name: "Confirmar restauração",
    }),
  );
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  expect(screen.getByRole("dialog")).toHaveAccessibleName(
    "Restauração · Preparando",
  );
  await user.click(screen.getByRole("button", { name: "Minimizar" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await user.click(screen.getByRole("link", { name: /Perfis de banco/ }));
  expect(
    screen.getByRole("heading", { name: "Perfis de banco" }),
  ).toBeInTheDocument();
  finish({ ...running, kind: "restore" });
  await waitFor(() => expect(api.events).toHaveBeenCalled());
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
