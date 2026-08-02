import CssBaseline from "@mui/material/CssBaseline";
import { ThemeProvider } from "@mui/material/styles";
import { StrictMode, useState } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import "./index.css";
import { darkTheme, lightTheme } from "./theme";

const storageKey = "patchcord-dashboard-theme-mode";

function Root() {
  const [mode, setMode] = useState<"light" | "dark">(() => {
    const stored = localStorage.getItem(storageKey);
    if (stored === "light" || stored === "dark") return stored;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });

  function toggleMode() {
    setMode((prev) => {
      const next = prev === "dark" ? "light" : "dark";
      localStorage.setItem(storageKey, next);
      return next;
    });
  }

  return (
    <ThemeProvider theme={mode === "dark" ? darkTheme : lightTheme}>
      <CssBaseline />
      <App mode={mode} onToggleMode={toggleMode} />
    </ThemeProvider>
  );
}

const rootEl = document.getElementById("root");
if (!rootEl) {
  throw new Error("#root element not found");
}

createRoot(rootEl).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
