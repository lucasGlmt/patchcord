import { createTheme, type ThemeOptions } from "@mui/material/styles";

function buildTheme(mode: "light" | "dark"): ThemeOptions {
  return {
    palette: {
      mode,
      primary: { main: "#5b5bd6" },
    },
    shape: { borderRadius: 8 },
    typography: {
      fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
      h1: undefined,
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
    },
  };
}

export const lightTheme = createTheme(buildTheme("light"));
export const darkTheme = createTheme(buildTheme("dark"));
