import type { Finding } from "../types";
import { findingLocation } from "../lib/format";
import { toCsv } from "../lib/csv";

function jiraPriority(severity: string): string {
  switch (severity) {
    case "critical": return "Highest";
    case "high": return "High";
    case "medium": return "Medium";
    case "low": return "Low";
    case "info": return "Lowest";
    default: return "Medium";
  }
}

function jiraIssueType(severity: string): string {
  return severity === "critical" || severity === "high" ? "Bug" : "Task";
}

/** Build Jira CSV string from findings. Columns: issueType, summary, description, priority, labels */
export function findingsToJiraCsv(findings: Finding[]): string {
  const header = ["issueType", "summary", "description", "priority", "labels"];
  const rows = findings.map((f) => {
    const msg = f.message ?? "";
    const summary = (f.title ?? f.rule_id ?? msg).slice(0, 255);
    const loc = findingLocation(f as never);
    const descParts = [msg, `Location: ${loc}`];
    if (f.remediation) descParts.push(`Remediation: ${f.remediation}`);
    if (f.rule_id) descParts.push(`Rule: ${f.rule_id}`);
    const description = descParts.join("\n\n");
    return [jiraIssueType(f.severity ?? ""), summary, description, jiraPriority(f.severity ?? ""), f.analyzer_id ?? ""];
  });
  return toCsv(header, rows);
}

/** Create a blob URL for a Jira CSV download; caller should revoke when done. */
export function createJiraCsvBlobUrl(findings: Finding[]): string {
  const csv = findingsToJiraCsv(findings);
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
  return URL.createObjectURL(blob);
}

/** Trigger a download of Jira CSV for given findings. */
export function downloadJiraCsv(findings: Finding[], filename = "jira-import.csv"): void {
  const url = createJiraCsvBlobUrl(findings);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
