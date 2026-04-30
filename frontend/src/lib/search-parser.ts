export interface ParsedSearch {
  text: string;
  filters: Record<string, string>;
}

/**
 * Parses search input into key search text and field:value filters.
 *
 * Examples:
 *   "Hubstaff" → { text: "Hubstaff", filters: {} }
 *   "globalProductName:Hubstaff" → { text: "", filters: { globalProductName: "Hubstaff" } }
 *   "site:Capterra some text" → { text: "some text", filters: { site: "Capterra" } }
 *   'status:"in progress"' → { text: "", filters: { status: "in progress" } }
 */
export function parseSearch(input: string): ParsedSearch {
  const filters: Record<string, string> = {};
  const textParts: string[] = [];

  // Regex: (\w+) captures field name, then :"([^"]*)" captures quoted value OR (\S+) captures unquoted
  const regex = /(\w+):(?:"([^"]*)"|(\S+))/g;
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
