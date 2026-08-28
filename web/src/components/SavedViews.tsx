import { useEffect, useMemo, useRef, useState } from 'react';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select';
import { Bookmark, Trash2, Share2, Check, Star, Pencil, Copy, Download, Upload } from 'lucide-react';
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
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

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

  const handleRename = (id: string) => {
    const n = editName.trim();
    if (!n) return;
    setViews((prev) => prev.map((v) => (v.id === id ? { ...v, name: n } : v)));
    setEditingId(null);
    setEditName('');
  };

  const handleDuplicate = (view: SavedView) => {
    const dup: SavedView = {
      id: `sv-${Date.now()}`,
      name: `${view.name} copy`,
      group: JSON.parse(JSON.stringify(view.group)),
      createdAt: new Date().toISOString(),
    };
    setViews((prev) => [...prev, dup]);
    setSelectedId(dup.id);
  };

  const handleExport = () => {
    const blob = new Blob([JSON.stringify(views, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `bluntcode-views-${new Date().toISOString().slice(0, 10)}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleImport = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const parsed = JSON.parse(String(reader.result));
        const arr: SavedView[] = Array.isArray(parsed) ? parsed : parsed.views ?? [];
        const valid = arr.filter((v: unknown) => v && typeof (v as SavedView).name === 'string' && (v as SavedView).group);
        if (valid.length === 0) return;
        const withIds = valid.map((v) => ({ ...v, id: `sv-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`, createdAt: v.createdAt ?? new Date().toISOString() }));
        setViews((prev) => [...prev, ...withIds]);
      } catch { /* ignore parse error */ }
      if (fileInputRef.current) fileInputRef.current.value = '';
    };
    reader.readAsText(file);
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
              {editingId === selectedView.id ? (
                <div className="flex flex-col gap-2">
                  <label htmlFor="rename-view" className="text-xs font-semibold text-[var(--color-ink)]">Rename view</label>
                  <input
                    id="rename-view"
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    onKeyDown={(e) => { if (e.key === 'Enter') handleRename(selectedView.id); if (e.key === 'Escape') setEditingId(null); }}
                    autoFocus
                    className="h-8 w-full rounded-[var(--radius-button)] border border-[var(--color-rule-strong)] bg-[var(--color-surface)] px-3 text-xs text-[var(--color-ink)] focus:outline-none focus:ring-2 focus:ring-[var(--color-focus)]"
                  />
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" className="flex-1" onClick={() => setEditingId(null)}>Cancel</Button>
                    <Button size="sm" className="flex-1" disabled={!editName.trim()} onClick={() => handleRename(selectedView.id)}>Save</Button>
                  </div>
                </div>
              ) : (
                <>
                  <p className="text-sm font-semibold text-[var(--color-ink)] inline-flex items-center gap-1.5">
                    {selectedView.isDefault && <Star className="h-3.5 w-3.5 fill-[var(--color-warning)] text-[var(--color-warning)]" aria-hidden="true" />}
                    {selectedView.name}
                  </p>
                  <p className="text-xs text-[var(--color-ink-faint)]">{selectedView.group.rows.length} condition(s) · {selectedView.group.logic}</p>
                  <div className="mt-3 flex flex-col gap-2">
                    <Button variant="outline" size="sm" onClick={() => handleShare(selectedView)}>
                      {copiedId === selectedView.id ? <Check className="h-3.5 w-3.5" /> : <Share2 className="h-3.5 w-3.5" />}
                      {copiedId === selectedView.id ? 'Copied' : 'Share link'}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => { setEditingId(selectedView.id); setEditName(selectedView.name); }} aria-label={`Rename ${selectedView.name}`}>
                      <Pencil className="h-3.5 w-3.5" /> Rename
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => handleDuplicate(selectedView)} aria-label={`Duplicate ${selectedView.name}`}>
                      <Copy className="h-3.5 w-3.5" /> Duplicate
                    </Button>
                    <Button variant={selectedView.isDefault ? 'secondary' : 'outline'} size="sm" onClick={() => handleDefault(selectedView.id)} aria-label={selectedView.isDefault ? 'Unpin default view' : 'Pin as default view'}>
                      <Star className={selectedView.isDefault ? 'fill-[var(--color-warning)] text-[var(--color-warning)]' : ''} />
                      {selectedView.isDefault ? 'Default ★' : 'Pin default'}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => handleDelete(selectedView.id)} className="text-[var(--color-danger)] hover:text-[var(--color-danger)]">
                      <Trash2 className="h-3.5 w-3.5" /> Delete
                    </Button>
                  </div>
                </>
              )}
            </PopoverContent>
          </Popover>
        )}
      </div>

      <div className="flex items-center gap-1.5 flex-wrap">
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
        <Button variant="ghost" size="sm" onClick={handleExport} disabled={views.length === 0} aria-label="Export views as JSON">
          <Download className="h-3.5 w-3.5" /> Export
        </Button>
        <Button variant="ghost" size="sm" onClick={() => fileInputRef.current?.click()} aria-label="Import views from JSON">
          <Upload className="h-3.5 w-3.5" /> Import
        </Button>
        <input ref={fileInputRef} type="file" accept="application/json,.json" className="hidden" onChange={handleImport} aria-hidden="true" tabIndex={-1} />
        {views.length > 0 && (
          <Badge variant="outline" className="font-mono text-[10px]">
            {views.length}
          </Badge>
        )}
      </div>
    </div>
  );
}
