import type { WorkflowInputDetail } from "@patchcord/sdk";
import FormControlLabel from "@mui/material/FormControlLabel";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";

import type { TypedInputValue } from "../lib/workflowInputs";

// One form control per declared input's type: a switch for "boolean", a
// select for "enum", a (possibly numeric) text field otherwise.
export default function WorkflowInputField({
  input,
  value,
  onChange,
}: {
  input: WorkflowInputDetail;
  value: TypedInputValue;
  onChange: (value: TypedInputValue) => void;
}) {
  const helperText = input.description;

  if (input.type === "boolean") {
    return (
      <FormControlLabel
        control={<Switch checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />}
        label={
          <Stack>
            <Typography variant="body2">{input.name}</Typography>
            {helperText && (
              <Typography variant="caption" color="text.secondary">
                {helperText}
              </Typography>
            )}
          </Stack>
        }
      />
    );
  }

  if (input.type === "enum") {
    return (
      <TextField
        select
        label={input.name}
        required={input.required}
        helperText={helperText}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {(input.enum ?? []).map((option) => (
          <MenuItem key={option} value={option}>
            {option}
          </MenuItem>
        ))}
      </TextField>
    );
  }

  return (
    <TextField
      label={input.name}
      type={input.type === "number" ? "number" : "text"}
      required={input.required}
      helperText={helperText}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}
