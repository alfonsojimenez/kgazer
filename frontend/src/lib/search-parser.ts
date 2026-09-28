export interface ParsedSearch {
  text: string;
  filters: Record<string, string>;
}

/**
 * Parses search input into key search text and field:value filters.
 *
 * Supports nested key paths using dot notation and array elements using []:
 *
 * Examples:
 *   "Hubstaff" → { text: "Hubstaff", filters: {} }
 *   "globalProductName:Hubstaff" → { text: "", filters: { globalProductName: "Hubstaff" } }
 *   "site:Capterra some text" → { text: "some text", filters: { site: "Capterra" } }
 *   'status:"in progress"' → { text: "", filters: { status: "in progress" } }
 *   "spendBreakdown.type:ca:click" → { text: "", filters: { "spendBreakdown.type": "ca:click" } }
 *   "spendBreakdown[].type:ca:click" → { text: "", filters: { "spendBreakdown[].type": "ca:click" } }
 *   "ownerId:2101498 spendBreakdown[].type:ca:click" → { text: "", filters: { ownerId: "2101498", "spendBreakdown[].type": "ca:click" } }
 */
export function parseSearch(input: string): ParsedSearch {
  const filters: Record<string, string> = {};
  const textParts: string[] = [];

  // Regex: field path supports dot-separated segments with optional [] (array notation)
  //   (\w+(?:\[\])?(?:\.\w+(?:\[\])?)*)  captures the field path
  //   :"([^"]*)"                         captures quoted value
  //   |(\S+)                              captures unquoted value
  const regex = /(\w+(?:\[\])?(?:\.\w+(?:\[\])?)*):(?:"([^"]*)"|(\S+))/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = regex.exec(input)) !== null) {
    const before = input.slice(lastIndex, match.index).trim();
    if (before) textParts.push(before);

    const field = match[1];
    const value = match[2] ?? match[3];
    filters[field] = value;
    lastIndex = regex.lastIndex;
  }

  const remaining = input.slice(lastIndex).trim();
  if (remaining) textParts.push(remaining);

  return {
    text: textParts.join(" "),
    filters,
  };
}

export function buildSearch(parsed: ParsedSearch): string {
  const parts: string[] = [];
  for (const [field, value] of Object.entries(parsed.filters)) {
    if (value.includes(" ")) {
      parts.push(`${field}:"${value}"`);
    } else {
      parts.push(`${field}:${value}`);
    }
  }
  if (parsed.text) parts.push(parsed.text);
  return parts.join(" ");
}

/**
 * Converts flat dotted-path filters into a nested JSON string for PostgreSQL's
 * JSONB containment operator (@>).
 *
 * Dot notation creates nested objects; [] creates array elements:
 *   { "ownerId": "2101498" }
 *     → {"ownerId":"2101498"}
 *   { "spendBreakdown.type": "ca:click" }
 *     → {"spendBreakdown":{"type":"ca:click"}}
 *   { "spendBreakdown[].type": "ca:click" }
 *     → {"spendBreakdown":[{"type":"ca:click"}]}
 *   { "ownerId": "2101498", "spendBreakdown[].type": "ca:click" }
 *     → {"ownerId":"2101498","spendBreakdown":[{"type":"ca:click"}]}
 */
export function filtersToJSON(filters: Record<string, string>): string {
  const result: Record<string, unknown> = {};
  for (const [path, value] of Object.entries(filters)) {
    const segments = path.split(".");
    const nested = buildNestedValue(segments, value) as Record<string, unknown>;
    deepMerge(result, nested);
  }
  return JSON.stringify(result);
}

function buildNestedValue(segments: string[], value: string): unknown {
  const seg = segments[0];
  const isArray = seg.endsWith("[]");
  const key = isArray ? seg.slice(0, -2) : seg;
  const rest = segments.slice(1);

  if (isArray) {
    if (rest.length === 0) {
      return { [key]: [value] };
    }
    return { [key]: [buildNestedValue(rest, value)] };
  }

  if (rest.length === 0) {
    return { [key]: value };
  }
  return { [key]: buildNestedValue(rest, value) };
}

function deepMerge(
  target: Record<string, unknown>,
  source: Record<string, unknown>,
): void {
  for (const [key, value] of Object.entries(source)) {
    if (Array.isArray(value)) {
      if (Array.isArray(target[key])) {
        (target[key] as unknown[]).push(...value);
      } else {
        target[key] = value;
      }
    } else if (typeof value === "object" && value !== null) {
      if (
        typeof target[key] === "object" &&
        target[key] !== null &&
        !Array.isArray(target[key])
      ) {
        deepMerge(target[key] as Record<string, unknown>, value as Record<string, unknown>);
      } else {
        target[key] = value;
      }
    } else {
      target[key] = value;
    }
  }
}
