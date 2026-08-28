import { useMemo, useState } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover';
import { Combobox, type ComboboxOption } from './ui/combobox';
import { cn } from '../lib/utils';
import { FIELD_LABELS, OP_LABELS, QUERY_FIELDS, QUERY_OPS, type QueryGroup, type QueryRow, nextRowId, previewSql, queryGroupToFilter, buildUrlSearchFromFilter } from '../lib/queryBuilder';
import type { FindingFilter, SortState } from '../pages/report/ReportView';
import { Trash2, Plus, Copy, Check } from 'lucide-react';
import { copyToClipboard } from '../lib/clipboard';

type Props = {
  group: QueryGroup;
  onChange: (g: QueryGroup) => void;
  onApply?: (f: FindingFilter) => void;
  facetCounts?: Record<string, Record<string, number>>;
  analyzers?: string[];
  rules?: string[];
  sort?: SortState;
  page?: number;
};

const SEVERITY_OPTS = ['critical', 'high', 'medium', 'low', 'info'];
const STATUS_OPTS = ['new', 'persistent', 'suppressed'];

function optionsForField(field: QueryRow['field'], analyzers: string[], rules: string[], counts?: Record<string, number>): ComboboxOption[] {
  let values: string[] = [];
  if (field === 'severity') values = SEVERITY_OPTS;
  else if (field === 'status') values = STATUS_OPTS;
  else if (field === 'analyzer') values = analyzers;
  else if (field === 'rule') values = rules;
  else values = [];
  // if no predefined list (path/category), return empty so free-text input shows
  return values.map((v) => {
    const c = counts?.[v];
    return { value: v, label: c != null ? `${v} (${c})` : v };
  });
}

export function QueryBuilder({ group, onChange, onApply, facetCounts, analyzers = [], rules = [], sort, page }: Props) {
  const [copied, setCopied] = useState(false);
  const preview = useMemo(() => previewSql(group), [group]);
  const urlSearch = useMemo(() => buildUrlSearchFromFilter(queryGroupToFilter(group), sort, page), [group, sort, page]);

  const updateRow = (id: string, patch: Partial<QueryRow>) => {
    onChange({ ...group, rows: group.rows.map((r) => (r.id === id ? { ...r, ...patch } : r)) });
  };
  const removeRow = (id: string) => {
    const next = group.rows.filter((r) => r.id !== id);
    onChange({ ...group, rows: next.length ? next : [{ id: nextRowId(), field: 'severity', op: '=', value: '' }] });
  };
  const addRow = () => onChange({ ...group, rows: [...group.rows, { id: nextRowId(), field: 'severity', op: '=', value: '' }] });

  const handleCopy = async () => {
    const url = `${window.location.pathname}${urlSearch ? `?${urlSearch}` : ''}${window.location.hash}`;
    if (await copyToClipboard(`${window.location.origin}${url}`)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2" role="group" aria-label="Match logic">
          <span className="text-xs font-mono font-semibold uppercase tracking-widest text-[var(--color-ink-faint)]">Match</span>
          <div className="flex rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface-muted)] p-0.5">
            {(['AND', 'OR'] as const).map((logic) => (
              <button
                key={logic}
                type="button"
                aria-pressed={group.logic === logic}
                onClick={() => onChange({ ...group, logic })}
                className={cn(
                  'rounded-[var(--radius-sm)] px-3 py-1 text-xs font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)]',
                  group.logic === logic ? 'bg-[var(--color-surface)] text-[var(--color-ink)] shadow-xs border border-[var(--color-rule)]' : 'text-[var(--color-ink-faint)] hover:text-[var(--color-ink)]',
                )}
              >
                {logic}
              </button>
            ))}
          </div>
        </div>
        <Button variant="outline" size="sm" onClick={addRow} aria-label="Add condition">
          <Plus className="h-3.5 w-3.5" /> Add row
        </Button>
      </div>

      <div className="flex flex-col gap-2" role="list" aria-label="Query conditions">
        {group.rows.map((row) => {
          const counts = facetCounts?.[row.field];
          const opts = optionsForField(row.field, analyzers, rules, counts);
          const isFreeText = opts.length === 0;
          return (
            <div key={row.id} role="listitem" className="flex items-center gap-2 rounded-[var(--radius-lg)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] p-2">
              <Select value={row.field} onValueChange={(v) => updateRow(row.id, { field: v as QueryRow['field'], value: '' })}>
                <SelectTrigger className="h-8 w-[130px] shrink-0 text-xs" aria-label="Field">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {QUERY_FIELDS.map((f) => (
                    <SelectItem key={f} value={f}>
                      {FIELD_LABELS[f]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={row.op} onValueChange={(v) => updateRow(row.id, { op: v as QueryRow['op'] })}>
                <SelectTrigger className="h-8 w-[130px] shrink-0 text-xs" aria-label="Operator">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {QUERY_OPS.map((op) => (
                    <SelectItem key={op} value={op}>
                      {OP_LABELS[op]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <div className="min-w-0 flex-1">
                {isFreeText ? (
                  <input
                    value={row.value}
                    onChange={(e) => updateRow(row.id, { value: e.target.value })}
                    placeholder={row.field === 'path' ? 'src/' : row.field === 'category' ? 'security' : 'value'}
                    aria-label={`Value for ${row.field}`}
                    className="h-8 w-full rounded-[var(--radius-button)] border border-[var(--color-rule-strong)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-ink)] placeholder:text-[var(--color-ink-faint)] focus:outline-none focus:ring-2 focus:ring-[var(--color-focus)] focus:border-[var(--color-accent)]"
                  />
                ) : (
                  <Combobox
                    options={opts}
                    value={row.value}
                    onValueChange={(v) => updateRow(row.id, { value: v })}
                    placeholder="Select…"
                    searchPlaceholder="Search…"
                    triggerClassName="h-8 text-xs"
                  />
                )}
              </div>

              <Button variant="ghost" size="icon" aria-label="Remove condition" onClick={() => removeRow(row.id)} className="h-8 w-8 shrink-0 text-[var(--color-ink-faint)] hover:text-[var(--color-danger)]">
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          );
        })}
      </div>

      <div className="rounded-[var(--radius-lg)] border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] p-3">
        <p className="text-[10px] font-mono font-semibold uppercase tracking-widest text-[var(--color-ink-faint)]">Preview</p>
        <p className="mt-1 font-mono text-xs text-[var(--color-ink)] break-all" aria-live="polite">
          {preview}
        </p>
        {urlSearch ? (
          <div className="mt-2 flex flex-wrap gap-1.5" aria-label="URL preview">
            <Badge variant="outline" className="font-mono text-[10px] max-w-full truncate">
              ?{urlSearch}
            </Badge>
          </div>
        ) : null}
      </div>

      <div className="flex flex-wrap gap-2">
        {onApply && (
          <Button
            size="sm"
            onClick={() => onApply(queryGroupToFilter(group))}
            className="motion-reduce:transition-none"
          >
            Apply filters
          </Button>
        )}
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="outline" size="sm" aria-label="Share query link">
              {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
              {copied ? 'Copied' : 'Copy link'}
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-80" align="start">
            <p className="text-xs font-semibold text-[var(--color-ink)]">Shareable link</p>
            <p className="mt-1 break-all font-mono text-xs text-[var(--color-ink-soft)]">{urlSearch ? `?${urlSearch}` : '(no filters)'}</p>
            <Button variant="outline" size="sm" className="mt-2 w-full" onClick={handleCopy}>
              {copied ? 'Copied!' : 'Copy full URL'}
            </Button>
          </PopoverContent>
        </Popover>
      </div>
    </div>
  );
}
