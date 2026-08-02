import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";

export default function JsonBlock({ label, value }: { label?: string; value: unknown }) {
  return (
    <Box sx={{ mt: label ? 1 : 0 }}>
      {label && (
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 0.25 }}>
          {label}
        </Typography>
      )}
      <Box
        component="pre"
        sx={{
          m: 0,
          p: 1,
          borderRadius: 1,
          bgcolor: (theme) => (theme.palette.mode === "dark" ? "rgba(255,255,255,0.06)" : "rgba(0,0,0,0.04)"),
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
          fontSize: 12.5,
          overflowX: "auto",
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
        }}
      >
        {typeof value === "string" ? value : JSON.stringify(value, null, 2)}
      </Box>
    </Box>
  );
}
