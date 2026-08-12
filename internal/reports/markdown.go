package reports

import (
	"bluntcode/internal/analyzers"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"
)

func Filename(workspace string, t time.Time) string {
	return "blunt-code-" + safe(workspace) + "-" + t.Format("20060102-150405") + ".md"
}

// Markdown produces one exportable report. Its tables intentionally follow
// the familiar SonarQube report shape while retaining the analyzer that
// produced every finding.
func Markdown(m Model) string {
	var b strings.Builder
	b.WriteString("# Blunt Code Analysis Report - " + escape(m.WorkspaceName) + "\n\n")
	b.WriteString("Generated on: " + m.StartedAt.UTC().Format("2006-01-02 15:04:05 UTC") + "\n\n")
	qualityMetrics(&b, m)
	findingsTable(&b, "Open Findings", m.Findings)
	findingsTable(&b, "Security Findings", byCategories(m.Findings, analyzers.CategorySecurity, analyzers.CategoryVulnerability))
	changeSummary(&b, m)
	scanDetails(&b, m)
	analyzerSummary(&b, m)
	section(&b, "Warnings and Incomplete Analysis", bullet(m.Warnings, "None."))
	section(&b, "Report Notes", "Generated locally by Blunt Code. Paths are workspace-relative; no source snippets are included.")
	return b.String()
}

func qualityMetrics(b *strings.Builder, m Model) {
	b.WriteString("## Quality Metrics Summary\n\n")
	b.WriteString("| Category | Metric | Value |\n| :--- | :--- | :--- |\n")
	metricRow(b, "**Size**", "Lines of Code (LOC)", metric(m, "ncloc", "N/A"))
	metricRow(b, "**Complexity**", "Cognitive Complexity", metric(m, "cognitive_complexity", "N/A"))
	metricRow(b, "", "Cyclomatic Complexity", metric(m, "complexity", "N/A"))
	metricRow(b, "**Issues**", "Total Findings", fmt.Sprintf("%d", len(m.Findings)))
	metricRow(b, "", "Bugs", countCategory(m.Findings, analyzers.CategoryBug))
	metricRow(b, "", "Vulnerabilities", countCategory(m.Findings, analyzers.CategoryVulnerability))
	metricRow(b, "", "Code Smells", countCategory(m.Findings, analyzers.CategoryCodeSmell))
	metricRow(b, "**Security & Ratings**", "Security Hotspots", metricOrCount(m, "security_hotspots", countCategory(m.Findings, analyzers.CategorySecurity)))
	metricRow(b, "", "Security Rating", rating(m, "security_rating"))
	metricRow(b, "", "Reliability Rating", rating(m, "reliability_rating"))
	metricRow(b, "", "Maintainability Rating", rating(m, "sqale_rating"))
	metricRow(b, "**Coverage & Duplication**", "Coverage", percent(m, "coverage"))
	metricRow(b, "", "Duplicated Lines Density", percent(m, "duplicated_lines_density"))
	b.WriteString("\n")
}

func metricRow(b *strings.Builder, category, name, value string) {
	b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", category, name, value))
}

func findingsTable(b *strings.Builder, title string, findings []analyzers.Finding) {
	b.WriteString("## " + title + "\n\n")
	if len(findings) == 0 {
		b.WriteString("[OK] No " + strings.ToLower(title) + " reported.\n\n")
		return
	}
	b.WriteString("| Tool | Severity | Type | File | Line | Rule | Message |\n| :--- | :--- | :--- | :--- | ---: | :--- | :--- |\n")
	for _, f := range findings {
		line := "N/A"
		if f.StartLine > 0 {
			line = fmt.Sprintf("%d", f.StartLine)
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
			cell(f.AnalyzerID), cell(displaySeverity(f)), cell(findingType(f)), cell(filepath.ToSlash(f.RelativePath)), line, cell(f.RuleID), cell(f.Message)))
	}
	b.WriteString("\n")
}

func changeSummary(b *strings.Builder, m Model) {
	b.WriteString("## Change Since Previous Scan\n\n")
	b.WriteString("| New | Fixed | Persistent |\n| ---: | ---: | ---: |\n")
	b.WriteString(fmt.Sprintf("| %d | %d | %d |\n\n", len(m.Comparison.New), len(m.Comparison.Fixed), len(m.Comparison.Persistent)))
}

func scanDetails(b *strings.Builder, m Model) {
	b.WriteString("## Scan Details\n\n")
	b.WriteString("| Field | Value |\n| :--- | :--- |\n")
	detailRow(b, "Workspace", cell(m.WorkspaceName))
	detailRow(b, "Scan", cell(m.ScanID))
	detailRow(b, "Profile", cell(m.Profile))
	detailRow(b, "Analyzed files", fmt.Sprintf("%d", len(m.Files)))
	detailRow(b, "Skipped files", fmt.Sprintf("%d", len(m.SkippedFiles)))
	detailRow(b, "Analysis completeness", map[bool]string{true: "PARTIAL", false: "COMPLETE"}[m.Partial])
	b.WriteString("\n")
}

func detailRow(b *strings.Builder, field, value string) {
	b.WriteString(fmt.Sprintf("| %s | %s |\n", field, value))
}

func analyzerSummary(b *strings.Builder, m Model) {
	b.WriteString("## Analyzer Summary\n\n")
	b.WriteString("| Analyzer | State | Version | Findings | Duration | Detail |\n| :--- | :--- | :--- | ---: | ---: | :--- |\n")
	for _, r := range m.Runs {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s | %s |\n", cell(display(r)), cell(r.State), cell(r.Version), r.FindingCount, duration(r.Duration), cell(r.ErrorSummary)))
	}
	b.WriteString("\n")
}

func metric(m Model, key, fallback string) string {
	for _, x := range m.Metrics {
		if x.Key == key {
			return number(x.Value)
		}
	}
	return fallback
}

func metricOrCount(m Model, key, fallback string) string {
	if value := metric(m, key, ""); value != "" {
		return value
	}
	return fallback
}

func percent(m Model, key string) string {
	if value := metric(m, key, ""); value != "" {
		return value + "%"
	}
	return "N/A"
}

func rating(m Model, key string) string {
	for _, x := range m.Metrics {
		if x.Key != key {
			continue
		}
		switch int(x.Value) {
		case 1:
			return "A"
		case 2:
			return "B"
		case 3:
			return "C"
		case 4:
			return "D"
		case 5:
			return "E"
		default:
			return number(x.Value)
		}
	}
	return "N/A"
}

func number(value float64) string {
	if math.Trunc(value) == value {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%g", value)
}

func countCategory(findings []analyzers.Finding, category analyzers.Category) string {
	count := 0
	for _, f := range findings {
		if f.Category == category {
			count++
		}
	}
	return fmt.Sprintf("%d", count)
}

func displaySeverity(f analyzers.Finding) string {
	if f.RawSeverity != "" {
		return f.RawSeverity
	}
	return string(f.Severity)
}

func findingType(f analyzers.Finding) string {
	if value, ok := f.Metadata["type"].(string); ok && value != "" {
		return value
	}
	return string(f.Category)
}

func duration(value time.Duration) string {
	if value <= 0 {
		return "N/A"
	}
	return value.Round(time.Millisecond).String()
}

func section(b *strings.Builder, title, body string) {
	b.WriteString("## " + title + "\n\n" + body + "\n\n")
}

func byCategories(fs []analyzers.Finding, c ...analyzers.Category) (o []analyzers.Finding) {
	for _, f := range fs {
		for _, x := range c {
			if f.Category == x {
				o = append(o, f)
				break
			}
		}
	}
	return
}

func bullet(xs []string, none string) string {
	if len(xs) == 0 {
		return none
	}
	return "- " + strings.Join(escapeAll(xs), "\n- ")
}

func escapeAll(xs []string) []string {
	o := make([]string, len(xs))
	for i, x := range xs {
		o[i] = escape(x)
	}
	return o
}

func cell(s string) string { return strings.ReplaceAll(escape(s), "|", "\\|") }

func escape(s string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "\r", " ", "\n", " ").Replace(s)
}

func safe(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	s = strings.Trim(b.String(), "-")
	if s == "" {
		return "workspace"
	}
	return s
}
