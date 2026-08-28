import { useEffect, useMemo, useState } from 'react';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { Bookmark, Trash2, Share2, Check, Star } from 'lucide-react';
import { copyToClipboard } from '../lib/clipboard';
import type { QueryGroup } from '../lib/queryBuilder';
import { buildUrlSearch } from '../lib/queryBuilder';
import type { SortState } from '../pages/report/ReportView';

const STORAGE_KEY = 'bluntcode.savedViews';

export type SavedView = { id: string; name: string; group: QueryGroup; createdAt: string; isDefault?: boolean };

function loadViews(): SavedView[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}
function saveViews(views: SavedView[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(views));
  } catch {}
}

export function SavedViews({
  group,
  onLoad,
  sort,
  page,
}: {
  group: QueryGroup;
  onLoad: (g: QueryGroup) => void;
  sort?: SortState;
  page?: number;
}) {
  const [views, setViews] = useState<SavedView[]>(() => loadViews());
  const [name, setName] = useState('');
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string>('');

  useEffect(() => {
    saveViews(views);
  }, [views]);

  const hasName = name.trim().length > 0;

  const handleSave = () => {
    const n = name.trim();
    if (!n) return;
    const existing = views.find((v) => v.name === n);
    const view: SavedView = {
      id: existing?.id ?? `sv-${Date.now()}`,
      name: n,
      group: JSON.parse(JSON.stringify(group)),
      createdAt: new Date().toISOString(),
      isDefault: existing?.isDefault,
    };
    setViews((prev) => {
      const filtered = prev.filter((v) => v.name !== n);
      return [...filtered, view];
    });
    setName('');
    setSelectedId(view.id);
  };

  const handleDelete = (id: string) => {
    setViews((prev) => prev.filter((v) => v.id !== id));
    if (selectedId === id) setSelectedId('');
  };

  const handleDefault = (id: string) => {
    setViews((prev) => prev.map((v) => ({ ...v, isDefault: v.id === id ? !v.isDefault : false })));
  };

  const handleShare = async (view: SavedView) => {
    const qs = buildUrlSearch(view.group, sort, page);
    const url = `${window.location.origin}${window.location.pathname}${qs ? `?${qs}` : ''}${window.location.hash}`;
    if (await copyToClipboard(url)) {
      setCopiedId(view.id);
      setTimeout(() => setCopiedId(null), 1200);
    }
  };

  const handleShareCurrent = async () => {
    const qs = buildUrlSearch(group, sort, page);
    const url = `${window.location.origin}${window.location.pathname}${qs ? `?${qs}` : ''}${window.location.hash}`;
    if (await copyToClipboard(url)) {
      setCopiedId('__current');
      setTimeout(() => setCopiedId(null), 1200);
    }
  };

  const selectedView = useMemo(() => views.find((v) => v.id === selectedId), [views, selectedId]);

  // auto-load default view on mount if no URL filters present
  useEffect(() => {
    const sp = new URLSearchParams(window.location.search);
    const hasFilters = ['severity', 'category', 'analyzer', 'rule', 'path', 'status', 'q'].some((k) => sp.get(k));
    if (hasFilters) return;
    const def = views.find((v) => v.isDefault);
    if (def) onLoad(def.group);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="flex items-center gap-2">
        <Select
          value={selectedId}
          onValueChange={(v) => {
            setSelectedId(v);
            const view = views.find((x) => x.id === v);
            if (view) onLoad(view.group);
          }}
        >
          <SelectTrigger className="h-8 w-[180px] text-xs" aria-label="Saved views">
            <SelectValue placeholder="Saved views" />
          </SelectTrigger>
          <SelectContent>
            {views.length === 0 ? (
              <div className="px-3 py-6 text-center text-xs text-[var(--color-ink-faint)]">No saved views</div>
            ) : (
              views.map((v) => (
                <SelectItem key={v.id} value={v.id}>
                  <span className="inline-flex items-center gap-1.5">
                    {v.isDefault && <Star className="h-3 w-3 fill-[var(--color-warning)] text-[var(--color-warning)]" aria-hidden="true" />}
                    {v.name}
                  </span>
                </SelectItem>
              ))
            )}
          </SelectContent>
        </Select>

        {selectedView && (
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="icon" className="h-8 w-8" aria-label={`Options for ${selectedView.name}`}>
                <Bookmark className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-64 p-3">
              <p className="text-sm font-semibold text-[var(--color-ink)]">{selectedView.name}</p>
              <p className="text-xs text-[var(--color-ink-faint)]">{selectedView.group.rows.length} condition(s) · {selectedView.group.logic}</p>
              <div className="mt-3 flex flex-col gap-2">
                <Button variant="outline" size="sm" onClick={() => handleShare(selectedView)}>
                  {copiedId === selectedView.id ? <Check className="h-3.5 w-3.5" /> : <Share2 className="h-3.5 w-3.5" />}
                  {copiedId === selectedView.id ? 'Copied' : 'Share link'}
                </Button>
                <Button variant={selectedView.isDefault ? 'secondary' : 'outline'} size="sm" onClick={() => handleDefault(selectedView.id)}>
                  <Star className={selectedView.isDefault ? 'fill-[var(--color-warning)] text-[var(--color-warning)]' : ''} />
                  {selectedView.isDefault ? 'Default' : 'Set default'}
                </Button>
                <Button variant="ghost" size="sm" onClick={() => handleDelete(selectedView.id)} className="text-[var(--color-danger)] hover:text-[var(--color-danger)]">
                  <Trash2 className="h-3.5 w-3.5" /> Delete
                </Button>
              </div>
            </PopoverContent>
          </Popover>
        )}
      </div>

      <div className="flex items-center gap-1.5">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="View name"
          aria-label="Saved view name"
          className="h-8 w-[140px] rounded-[var(--radius-button)] border border-[var(--color-rule-strong)] bg-[var(--color-surface)] px-3 text-xs text-[var(--color-ink)] placeholder:text-[var(--color-ink-faint)] focus:outline-none focus:ring-2 focus:ring-[var(--color-focus)]"
        />
        <Button variant="outline" size="sm" disabled={!hasName} onClick={handleSave} aria-label="Save current view">
          <Bookmark className="h-3.5 w-3.5" /> Save
        </Button>
        <Button variant="ghost" size="sm" onClick={handleShareCurrent} aria-label="Copy shareable link">
          {copiedId === '__current' ? <Check className="h-3.5 w-3.5" /> : <Share2 className="h-3.5 w-3.5" />}
          Share
        </Button>
        {views.length > 0 && (
          <Badge variant="outline" className="font-mono text-[10px]">
            {views.length}
          </Badge>
        )}
      </div>
    </div>
  );
}
