import { createTheme, type ThemeOptions } from "@mui/material/styles";

// Kept short and quick everywhere on purpose (150-250ms): a short-form demo
// video reads a quick, purposeful transition as polish and a slow one as
// sluggishness. Shared here so every framer-motion usage in src/motion.ts
// and the pages agrees with MUI's own transition durations.
export const motionDurationMs = 200;

function buildTheme(mode: "light" | "dark"): ThemeOptions {
  const isDark = mode === "dark";
  return {
    palette: {
      mode,
      primary: { main: "#5b5bd6" },
      secondary: { main: "#12b886" },
      background: isDark ? { default: "#0e0f13", paper: "#15171d" } : { default: "#f7f7fb", paper: "#ffffff" },
    },
    shape: { borderRadius: 10 },
    typography: {
      fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
      h1: undefined,
      h6: { fontWeight: 700, letterSpacing: -0.2 },
    },
    transitions: {
      duration: {
        shortest: 120,
        shorter: 150,
        short: motionDurationMs,
        standard: motionDurationMs,
        complex: 250,
      },
    },
    components: {
      MuiPaper: {
        defaultProps: { elevation: 0 },
        styleOverrides: {
          root: { backgroundImage: "none" },
        },
      },
      MuiAppBar: {
        defaultProps: { elevation: 0 },
      },
      MuiDrawer: {
        defaultProps: { elevation: 0 },
        styleOverrides: {
          paper: { backgroundImage: "none" },
        },
      },
    },
  };
}

export const lightTheme = createTheme(buildTheme("light"));
export const darkTheme = createTheme(buildTheme("dark"));
