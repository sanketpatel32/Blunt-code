import type { ReactNode } from 'react';
import { MoreHorizontal } from 'lucide-react';
import { Button } from './ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/dropdown-menu';

export interface RowMenuItem {
  label: string;
  onSelect: () => void;
  /** `danger` renders the item in the destructive red — reserved for Remove/Delete/etc. */
  tone?: 'default' | 'danger';
  disabled?: boolean;
  icon?: ReactNode;
}

export interface RowMenuProps {
  items: RowMenuItem[];
  /** Accessible name for the trigger; defaults to a generic label — pass the row's subject (e.g. "Actions for claire-frontend") when one is at hand. */
  label?: string;
  align?: 'start' | 'center' | 'end';
}

/** Overflow menu for row-level actions. The action-hierarchy contract keeps
 *  ONE visible action per row (the primary forward action); everything else —
 *  compare, export, rename, delete — lives in here, destructive items last so
 *  a stray click cannot reach them. */
export function RowMenu({ items, label = 'More actions', align = 'end' }: RowMenuProps) {
  if (!items.length) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0 text-[var(--color-ink-faint)] hover:text-[var(--color-ink)]"
          aria-label={label}
          title={label}
        >
          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align}>
        {items.map((item) => (
          <DropdownMenuItem
            key={item.label}
            onClick={item.onSelect}
            disabled={item.disabled}
            className={`gap-2 cursor-pointer ${item.tone === 'danger' ? 'text-[var(--color-danger)] focus:text-[var(--color-danger)]' : ''}`}
          >
            {item.icon}
            <span className="font-medium">{item.label}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
