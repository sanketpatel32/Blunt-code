/** Shared RFC4180 CSV field quoting. Reused by suppressions and Jira exports. */
export function csvField(value: string): string {
  return /[",\n\r]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value;
}

/** Build a CSV string with BOM from header + rows. */
export function toCsv(header: string[], rows: string[][]): string {
  const lines = [header, ...rows];
  return `\ufeff${lines.map((line) => line.map(csvField).join(",")).join("\n")}`;
}
