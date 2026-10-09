import { createContext, useContext } from "react";
import { kinds } from "./ui";
import type { JobKind } from "../types";
export type OperationKind = JobKind | "diagnostics" | "legacy_import";
export const operationNames: Record<OperationKind, string> = {
  ...kinds,
  diagnostics: "Verificação do ambiente",
  legacy_import: "Importação",
};
export type OperationRunner = <T>(
  kind: OperationKind,
  task: () => Promise<T>,
  resultMessage?: (result: T) => string,
) => Promise<T>;
const context = createContext<OperationRunner>(async (_kind, task) => task());
export const OperationFeedbackProvider = context.Provider;
export const useOperationRequest = () => useContext(context);
export interface OperationFeedback {
  id: number;
  kind: OperationKind;
  phase: "starting" | "succeeded" | "failed" | "cancelled";
  message: string;
}
