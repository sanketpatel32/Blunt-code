package reports

import (
	"bluntcode/internal/analyzers"
	"fmt"
	"html/template"
	"path/filepath"
	"sort"
	"strings"
)

// htmlDocument is the whole report: one file, no external assets, and exactly
// one inline script (htmlScript) that powers the client-side filters.
// Everything dynamic goes through html/template actions so contextual
// auto-escaping applies to workspace names, analyzer messages, rule ids, paths,
// and documentation URLs alike — including the data-* attributes the script
// reads back at run time. The <style> block is static CSS only.
const htmlDocument = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Blunt Code report — {{.WorkspaceName}}{{if .Date}} — {{.Date}}{{end}}</title>
<style>
:root{--ink:#111827;--muted:#6b7280;--line:#e5e7eb;--page:#ffffff;--critical:#b91c1c;--high:#dc2626;--medium:#b45309;--low:#4b5563;--info:#0f766e}
*{box-sizing:border-box}
body{margin:0;padding:2rem 1rem 3rem;font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;color:var(--ink);background:var(--page);line-height:1.5}
main{max-width:64rem;margin:0 auto}
h1{font-size:1.45rem;margin:0}
h2{font-size:.95rem;margin:2rem 0 .6rem;letter-spacing:.06em;text-transform:uppercase;color:var(--muted)}
.meta{display:flex;flex-wrap:wrap;gap:.25rem 1.5rem;margin:.6rem 0 0;font-size:.9rem;color:var(--muted)}
.meta strong{color:var(--ink);font-weight:600}
.meta span,.meta code{word-break:break-word}
.summary{display:flex;flex-wrap:wrap;gap:.6rem;margin-top:1.5rem}
.stat{min-width:6.5rem;border:1px solid var(--line);border-radius:.5rem;padding:.6rem .9rem;background:var(--page)}
.stat-value{display:block;font-size:1.5rem;font-weight:700;line-height:1.2}
.stat-label{display:block;font-size:.75rem;text-transform:uppercase;letter-spacing:.05em;color:var(--muted)}
.stat-critical .stat-value{color:var(--critical)}
.stat-high .stat-value{color:var(--high)}
.stat-medium .stat-value{color:var(--medium)}
.stat-low .stat-value{color:var(--low)}
.stat-info .stat-value{color:var(--info)}
.warnings{margin-top:1.5rem;border:1px solid #fde68a;border-left:4px solid var(--medium);border-radius:.5rem;padding:.75rem 1rem;background:#fffbeb}
.warnings h2{margin:0 0 .4rem}
.warnings ul{margin:0;padding-left:1.2rem}
.table-wrap{overflow-x:auto;border:1px solid var(--line);border-radius:.5rem}
table{width:100%;border-collapse:collapse;font-size:.9rem}
th{ text-align:left;font-size:.75rem;text-transform:uppercase;letter-spacing:.05em;color:var(--muted);border-bottom:2px solid var(--line);padding:.5rem .75rem;white-space:nowrap}
td{border-bottom:1px solid var(--line);padding:.55rem .75rem;vertical-align:top}
tbody tr:last-child td{border-bottom:none}
td.num,th.num{text-align:right;font-variant-numeric:tabular-nums}
.badge{display:inline-block;padding:.05rem .5rem;border-radius:999px;font-size:.72rem;font-weight:600;text-transform:uppercase;letter-spacing:.04em;border:1px solid transparent;white-space:nowrap}
.badge-critical{color:var(--critical);background:#fef2f2;border-color:#fecaca}
.badge-high{color:var(--high);background:#fef2f2;border-color:#fecaca}
.badge-medium{color:var(--medium);background:#fffbeb;border-color:#fde68a}
.badge-low{color:var(--low);background:#f9fafb;border-color:var(--line)}
.badge-info{color:var(--info);background:#f0fdfa;border-color:#99f6e4}
.tag{display:inline-block;padding:.05rem .5rem;border-radius:.3rem;font-size:.75rem;font-weight:600;background:#f3f4f6;border:1px solid var(--line);white-space:nowrap}
.rule-title{font-weight:600}
.rule-id{font-size:.8rem;color:var(--muted)}
.remediation{margin:.3rem 0 0;color:var(--muted)}
.remediation strong{font-weight:600}
code.path{font-family:ui-monospace,SFMono-Regular,Consolas,"Courier New",monospace;font-size:.82rem;white-space:nowrap}
a{color:#1d4ed8}
.empty{margin:.5rem 0 0;padding:.9rem 1rem;border:1px solid var(--line);border-radius:.5rem;color:var(--muted)}
.filters{display:flex;flex-wrap:wrap;gap:.6rem .9rem;align-items:flex-end;margin:0 0 1rem}
.filter-group{display:flex;flex-direction:column;gap:.3rem}
.filter-label{font-size:.72rem;font-weight:600;text-transform:uppercase;letter-spacing:.05em;color:var(--muted)}
.chip{font:inherit;font-size:.8rem;font-weight:600;color:var(--muted);background:var(--page);border:1px solid var(--line);border-radius:999px;padding:.25rem .7rem;cursor:pointer}
.chip.is-active{color:var(--ink);background:#f9fafb;border-color:#9ca3af}
.chip-critical.is-active{color:var(--critical);border-color:var(--critical)}
.chip-high.is-active{color:var(--high);border-color:var(--high)}
.chip-medium.is-active{color:var(--medium);border-color:var(--medium)}
.chip-low.is-active{color:var(--low);border-color:var(--low)}
.chip-info.is-active{color:var(--info);border-color:var(--info)}
.chip-count{margin-left:.15rem;font-weight:700;font-variant-numeric:tabular-nums}
select,input[type="search"]{font:inherit;font-size:.85rem;color:var(--ink);background:var(--page);border:1px solid var(--line);border-radius:.4rem;padding:.3rem .5rem;max-width:14rem}
.filter-clear{font:inherit;font-size:.85rem;color:var(--ink);background:var(--page);border:1px solid var(--line);border-radius:.4rem;padding:.32rem .7rem;cursor:pointer}
.filter-status{margin:0 0 0 auto;align-self:center;font-size:.85rem;color:var(--muted);font-variant-numeric:tabular-nums}
:focus-visible{outline:2px solid #1d4ed8;outline-offset:2px}
tr[hidden]{display:none}
details.file-group{border:1px solid var(--line);border-radius:.5rem;margin:0 0 .75rem}
details.file-group[hidden]{display:none}
.file-group summary{display:flex;flex-wrap:wrap;gap:.25rem .6rem;align-items:baseline;cursor:pointer;padding:.5rem .75rem}
.group-count{font-size:.78rem;color:var(--muted)}
footer{margin-top:2.5rem;padding-top:1rem;border-top:1px solid var(--line);font-size:.85rem;color:var(--muted)}
@media (max-width:40rem){body{padding:1rem .75rem 2rem}.stat{min-width:calc(50% - .6rem);flex:1}}
@media print{
 body{padding:0;color:#000}
 .summary{gap:.4rem}
 .stat,.table-wrap,.warnings,.empty{border-color:#999}
 .badge,.tag{background:transparent;border:1px solid currentColor}
 .stat-critical .stat-value,.stat-high .stat-value{color:#000;font-weight:700}
 a{color:#000;text-decoration:none}
 tr{break-inside:avoid;page-break-inside:avoid}
 .table-wrap{overflow:visible}
 h2{break-after:avoid}
 .filters{display:none}
 details.file-group{border-color:#999;break-inside:avoid;page-break-inside:avoid}
}
body.is-printing .filters,body.is-printing #filter-no-match{display:none}
body.is-printing tr[hidden]{display:table-row}
body.is-printing details.file-group[hidden]{display:block}
</style>
</head>
<body>
<main>
<header>
<h1>Blunt Code — {{.WorkspaceName}}</h1>
<p class="meta">
{{- if .WorkspacePath}}<span><strong>Root:</strong> {{.WorkspacePath}}</span>{{end -}}
{{- if .Profile}}<span><strong>Profile:</strong> {{.Profile}}</span>{{end -}}
{{- if .Started}}<span><strong>Started:</strong> {{.Started}}</span>{{end -}}
{{- if .Finished}}<span><strong>Finished:</strong> {{.Finished}}</span>{{end -}}
</p>
</header>
<section class="summary" aria-label="Findings summary">
<div class="stat"><span class="stat-value">{{.Total}}</span><span class="stat-label">Total</span></div>
<div class="stat"><span class="stat-value">{{.New}}</span><span class="stat-label">New</span></div>
<div class="stat"><span class="stat-value">{{.Fixed}}</span><span class="stat-label">Fixed</span></div>
<div class="stat"><span class="stat-value">{{.Persistent}}</span><span class="stat-label">Persistent</span></div>
{{range .Severities}}<div class="stat stat-{{.Class}}"><span class="stat-value">{{.Count}}</span><span class="stat-label">{{.Name}}</span></div>
{{end}}</section>
{{if .Warnings}}<section class="warnings">
<h2>Warnings — incomplete analysis</h2>
<ul>{{range .Warnings}}<li>{{.}}</li>{{end}}</ul>
</section>
{{end}}<section>
<h2>Analyzers</h2>
<div class="table-wrap">
<table>
<thead><tr><th>Analyzer</th><th>Status</th><th class="num">Findings</th><th class="num">Duration</th></tr></thead>
<tbody>
{{range .Runs}}<tr><td>{{.Name}}</td><td>{{.State}}</td><td class="num">{{.Findings}}</td><td class="num">{{.Duration}}</td></tr>
{{else}}<tr><td colspan="4">No analyzer runs recorded.</td></tr>{{end}}
</tbody>
</table>
</div>
</section>
<section id="findings">
<h2>Findings</h2>
{{if .Findings}}<div class="filters">
<div class="filter-group" role="group" aria-label="Filter findings by severity">
{{range .Severities}}<button type="button" class="chip chip-{{.Class}} is-active" data-sev="{{.Class}}" aria-pressed="true">{{.Name}} <span class="chip-count">{{.Count}}</span></button>
{{end}}</div>
<div class="filter-group">
<label class="filter-label" for="filter-analyzer">Analyzer</label>
<select id="filter-analyzer">
<option value="">All analyzers</option>
{{range .Analyzers}}<option value="{{.}}">{{.}}</option>
{{end}}</select>
</div>
<div class="filter-group">
<label class="filter-label" for="filter-search">Search</label>
<input type="search" id="filter-search" placeholder="Rule, message, or file" autocomplete="off">
</div>
<button type="button" class="filter-clear" id="filter-clear">Clear filters</button>
<p class="filter-status" id="filter-status" role="status" aria-live="polite">Showing {{.Total}} of {{.Total}} findings</p>
</div>
<p class="empty" id="filter-no-match" hidden>No findings match the current filters.</p>
{{range .Groups}}<details class="file-group" data-file="{{.Path}}" open>
<summary><code class="path">{{.Path}}</code> <span class="group-count">{{.Label}}</span></summary>
<div class="table-wrap">
<table>
<thead><tr><th>Severity</th><th>Rule</th><th>Message</th><th class="num">Line</th><th>Analyzer</th></tr></thead>
<tbody>
{{range .Findings}}<tr data-sev="{{.Class}}" data-an="{{.Analyzer}}" data-rule="{{.Rule}}" data-file="{{.File}}">
<td><span class="badge badge-{{.Class}}">{{.Severity}}</span></td>
<td>{{if .Title}}<span class="rule-title">{{.Title}}</span><br>{{end}}<span class="rule-id">{{if .DocsURL}}<a href="{{.DocsURL}}" rel="noopener noreferrer" target="_blank">{{.Rule}}</a>{{else}}{{.Rule}}{{end}}</span></td>
<td>{{if .Message}}{{.Message}}{{else}}—{{end}}{{if .Remediation}}<div class="remediation"><strong>Fix:</strong> {{.Remediation}}</div>{{end}}</td>
<td class="num">{{if .Line}}{{.Line}}{{else}}—{{end}}</td>
<td>{{if .Analyzer}}<span class="tag">{{.Analyzer}}</span>{{else}}—{{end}}</td>
</tr>
{{end}}</tbody>
</table>
</div>
</details>
{{end}}{{else}}<p class="empty">No findings — nice work.</p>
{{end}}</section>
<footer>Generated locally by Blunt Code{{if .Version}} v{{.Version}}{{end}} — code never left this computer.</footer>
</main>
<script>
{{.Script}}
</script>
</body>
</html>
`

// htmlTemplate is parsed once at startup; template.Must makes a malformed
// template a build-time failure rather than a per-report error.
var htmlTemplate = template.Must(template.New("report").Parse(htmlDocument))

type htmlModel struct {
	WorkspaceName, WorkspacePath, Profile, Date, Started, Finished, Version string
	Total, New, Fixed, Persistent                                           int
	Severities                                                              []htmlSeverity
	Warnings                                                                []string
	Runs                                                                    []htmlRun
	Findings                                                                []htmlFinding
	Groups                                                                  []htmlGroup
	Analyzers                                                               []string
	Script                                                                  template.JS
}
type htmlSeverity struct {
	Name, Class string
	Count       int
}
type htmlRun struct {
	Name, State, Duration string
	Findings              int
}
type htmlFinding struct {
	Class, Severity, Title, Rule, Message, Remediation, Analyzer, DocsURL, File string
	Line                                                                        int
}

// htmlGroup buckets the findings of one file into a collapsible section; Path
// doubles as the deterministic sort key and Label carries the pre-pluralized
// count so the template never formats.
type htmlGroup struct {
	Path, Label string
	Findings    []htmlFinding
}

// HTML renders the report model as a standalone, self-contained HTML document
// that can be shared, archived, or printed with zero dependencies. Every
// user-controlled value passes through html/template contextual auto-escaping;
// nothing is concatenated into markup by hand. The document ships one static
// inline script (htmlScript) whose only job is toggling the visibility of the
// server-rendered finding rows; it never builds HTML from finding text.
func HTML(m Model) []byte {
	view := htmlModel{
		WorkspaceName: scrubControls(m.WorkspaceName), WorkspacePath: scrubControls(m.WorkspacePath), Profile: scrubControls(m.Profile), Version: m.BluntCodeVersion,
		Total: len(m.Findings), New: len(m.Comparison.New), Fixed: len(m.Comparison.Fixed), Persistent: len(m.Comparison.Persistent),
		Script: template.JS(htmlScript),
	}
	for _, warning := range m.Warnings {
		view.Warnings = append(view.Warnings, scrubControls(warning))
	}
	if !m.StartedAt.IsZero() {
		view.Date = m.StartedAt.UTC().Format("2006-01-02")
		view.Started = m.StartedAt.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	if !m.FinishedAt.IsZero() {
		view.Finished = m.FinishedAt.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	for _, s := range []struct {
		name string
		sev  analyzers.Severity
	}{
		{"Critical", analyzers.SeverityCritical}, {"High", analyzers.SeverityHigh}, {"Medium", analyzers.SeverityMedium},
		{"Low", analyzers.SeverityLow}, {"Info", analyzers.SeverityInfo},
	} {
		view.Severities = append(view.Severities, htmlSeverity{Name: s.name, Class: severityClass(s.sev), Count: m.Counts[s.sev]})
	}
	for _, r := range m.Runs {
		view.Runs = append(view.Runs, htmlRun{Name: scrubControls(display(r)), State: scrubControls(r.State), Duration: duration(r.Duration), Findings: r.FindingCount})
	}
	for _, f := range m.Findings {
		title := strings.TrimSpace(f.Title)
		if title == f.RuleID {
			title = "" // some analyzers repeat the rule id as title; show it once
		}
		view.Findings = append(view.Findings, htmlFinding{
			Class: severityClass(f.Severity), Severity: string(f.Severity), Title: scrubControls(title),
			Rule: scrubControls(f.RuleID), Message: scrubControls(f.Message), Remediation: scrubControls(strings.TrimSpace(f.Remediation)),
			File: scrubControls(htmlFilePath(f)), Analyzer: scrubControls(f.AnalyzerID), DocsURL: scrubControls(strings.TrimSpace(f.DocumentationURL)),
			Line: f.StartLine,
		})
	}
	view.Groups = htmlGroups(view.Findings)
	view.Analyzers = htmlAnalyzerOptions(m)
	var b strings.Builder
	if err := htmlTemplate.Execute(&b, view); err != nil {
		// Unreachable with this view model: the template parses at startup and
		// every action prints a plain field. Keep the fallback a valid document.
		return []byte("<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>Blunt Code report</title></head><body><p>Report rendering failed.</p></body></html>")
	}
	return []byte(b.String())
}

// severityClass maps a normalized severity onto the fixed badge class set so a
// stray severity value can never become an arbitrary attribute value; unknown
// severities stay styled as low.
func severityClass(severity analyzers.Severity) string {
	switch severity {
	case analyzers.SeverityCritical:
		return "critical"
	case analyzers.SeverityHigh:
		return "high"
	case analyzers.SeverityMedium:
		return "medium"
	case analyzers.SeverityInfo:
		return "info"
	default:
		return "low"
	}
}

// htmlFilePath normalizes the workspace-relative path used both for grouping
// findings into file sections and for display; project-level findings keep a
// word instead of a bare "." so the cell is never empty or cryptic.
func htmlFilePath(f analyzers.Finding) string {
	path := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(f.RelativePath)), "./")
	if path == "" || path == "." {
		return "project"
	}
	return path
}

// htmlGroups buckets the rendered findings by file path, preserving the model's
// order inside each group and sorting the groups by path so identical input
// always renders byte-identical HTML.
func htmlGroups(findings []htmlFinding) []htmlGroup {
	var groups []htmlGroup
	index := map[string]int{}
	for _, f := range findings {
		i, ok := index[f.File]
		if !ok {
			groups = append(groups, htmlGroup{Path: f.File})
			i = len(groups) - 1
			index[f.File] = i
		}
		groups[i].Findings = append(groups[i].Findings, f)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Path < groups[j].Path })
	for i := range groups {
		groups[i].Label = fmt.Sprintf("%d findings", len(groups[i].Findings))
		if len(groups[i].Findings) == 1 {
			groups[i].Label = "1 finding"
		}
	}
	return groups
}

// htmlAnalyzerOptions lists the analyzer ids present in runs or findings for
// the analyzer filter dropdown, deduplicated and sorted. Ids are scrubbed
// before deduplication so two hostile ids that scrub to the same text collapse
// into one option, and the template escapes them again in both the value
// attribute and the option label.
func htmlAnalyzerOptions(m Model) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		id := scrubControls(strings.TrimSpace(raw))
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, r := range m.Runs {
		add(r.AnalyzerID)
	}
	for _, f := range m.Findings {
		add(f.AnalyzerID)
	}
	sort.Strings(out)
	return out
}

// isHostileControl reports whether a rune is a C0 control (other than the
// whitespace trio tab, LF, CR) or DEL.
func isHostileControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f
}

// scrubControls replaces NUL, BEL, ANSI escape sequences, and the other
// non-whitespace C0 controls and DEL with spaces before text reaches
// html/template. Contextual auto-escaping neutralizes markup characters but
// passes these raw controls through in text contexts (verified against
// Go's template package), and analyzer output is untrusted: a NUL smuggled
// into a finding message must not land as a raw byte in the exported file.
// Tab, LF, and CR are kept: HTML collapses them to whitespace and keeping
// them preserves the message text verbatim. Invalid UTF-8 bytes decode to
// and are rewritten as U+FFFD, so the output is always valid UTF-8.
func scrubControls(s string) string {
	if !strings.ContainsFunc(s, isHostileControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isHostileControl(r) {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}
