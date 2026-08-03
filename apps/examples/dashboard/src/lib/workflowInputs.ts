import type { WorkflowInputDetail } from "@patchcord/sdk";

export function parseJSONRecord(raw: string, fieldName: string): Record<string, unknown> {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return {};
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch (err) {
    throw new Error(`${fieldName} is not valid JSON: ${(err as Error).message}`);
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error(`${fieldName} must be a JSON object`);
  }
  return parsed as Record<string, unknown>;
}

// A boolean input always has a value (its switch is on or off), so it's
// stored as-is; every other declared type is edited as text (a select for
// "enum") and only converted to its declared type in buildTypedInputs, so a
// required-but-empty string/number/enum field can be told apart from "0" or
// "false".
export type TypedInputValue = string | boolean;

export function initialTypedInputs(inputs: WorkflowInputDetail[]): Record<string, TypedInputValue> {
  const values: Record<string, TypedInputValue> = {};
  for (const input of inputs) {
    if (input.type === "boolean") {
      values[input.name] = typeof input.default === "boolean" ? input.default : false;
    } else {
      values[input.name] = input.default !== undefined ? String(input.default) : "";
    }
  }
  return values;
}

// Converts the form's string/boolean state back into the typed record
// client.workflows.run expects, validating required fields along the way.
// Coercion mirrors internal/workflow.PrepareInputs (number/boolean parsing)
// since this is the same declared schema, just enforced client-side first
// for a fast, in-form error instead of a round trip to get a 400.
export function buildTypedInputs(
  inputs: WorkflowInputDetail[],
  values: Record<string, TypedInputValue>,
): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const input of inputs) {
    const raw = values[input.name];

    if (input.type === "boolean") {
      result[input.name] = raw;
      continue;
    }

    const text = typeof raw === "string" ? raw.trim() : "";
    if (text === "") {
      if (input.required) {
        throw new Error(`"${input.name}" is required`);
      }
      continue;
    }

    if (input.type === "number") {
      const n = Number(text);
      if (Number.isNaN(n)) {
        throw new Error(`"${input.name}" must be a number`);
      }
      result[input.name] = n;
    } else {
      result[input.name] = text;
    }
  }
  return result;
}
