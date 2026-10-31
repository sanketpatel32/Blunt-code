/**
 * Features that are built but not yet trustworthy enough to show.
 *
 * Each flag hides one surface everywhere it appears (nav, command palette,
 * tabs, routes). The code behind it, front and back end, stays in place, so
 * re-enabling a feature is a one-line change here.
 */
export const FEATURES = {
  /** Pentest page: the OWASP ZAP, Nuclei and Burp template-library tabs. */
  pentestTemplateLibraries: false,
  /** Pentest page: the per-test "Execute Check" runner in the OWASP suite. */
  pentestExecuteCheck: false,
  /** The /rules page (custom YAML rule drafts, which scans do not execute yet). */
  ruleStudio: false,
  /** Workspace overview: the Dependencies insight tab. */
  workspaceDependencies: false,
} as const;
