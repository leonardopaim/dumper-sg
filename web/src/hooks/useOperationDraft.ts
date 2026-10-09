import { useEffect, useState } from "react";

const noTransientFields: never[] = [];
export const backupDraftKey = "dumpersg.backupDraft";
export const restoreDraftKey = "dumpersg.restoreDraft";

// Persist only operation inputs, never a connection profile or its credentials.
export function useOperationDraft<T extends object>(
  key: string,
  defaults: () => T,
  transient: readonly (keyof T)[] = noTransientFields,
) {
  const [form, setForm] = useState<T>(() => {
    const result = defaults();
    try {
      const saved: unknown = JSON.parse(localStorage.getItem(key) || "null");
      if (!saved || typeof saved !== "object" || Array.isArray(saved))
        return result;
      for (const field of Object.keys(result) as (keyof T)[]) {
        const value = (saved as Record<keyof T, unknown>)[field];
        if (
          !transient.includes(field) &&
          typeof value === typeof result[field] &&
          (typeof value !== "number" || Number.isFinite(value))
        )
          result[field] = value as T[keyof T];
      }
    } catch {
      // Invalid or unavailable browser storage falls back to the form defaults.
    }
    return result;
  });
  useEffect(() => {
    try {
      const saved = Object.fromEntries(
        Object.entries(form).filter(
          ([field]) => !transient.includes(field as keyof T),
        ),
      );
      localStorage.setItem(key, JSON.stringify(saved));
    } catch {
      // Editing and executing remain available if storage is blocked or full.
    }
  }, [key, form, transient]);
  return [form, setForm] as const;
}
