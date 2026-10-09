import { useLayoutEffect, useState } from "react";

export type Theme = "light" | "dark";
export const themePreferenceKey = "dumpersg.theme";

function initialTheme(): Theme {
  try {
    return localStorage.getItem(themePreferenceKey) === "dark" ? "dark" : "light";
  } catch {
    return "light";
  }
}

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(initialTheme);

  useLayoutEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  const changeTheme = (value: Theme) => {
    setTheme(value);
    try {
      localStorage.setItem(themePreferenceKey, value);
    } catch {
      /* O tema continua funcionando quando o armazenamento está bloqueado. */
    }
  };

  return { theme, changeTheme };
}
