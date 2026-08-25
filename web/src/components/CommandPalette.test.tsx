import { act, useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CommandPalette, filterCommands, type Command } from './CommandPalette';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | undefined;
let container: HTMLDivElement | undefined;

afterEach(() => {
  if (root) act(() => root!.unmount());
  root = undefined;
  container?.remove();
  container = undefined;
});

const commands: Command[] = [
  { id: 'home', label: 'Go to Home', keywords: 'dashboard', run: vi.fn() },
  { id: 'tools', label: 'Go to Tools', keywords: 'ruff biome', run: vi.fn() },
  { id: 'theme', label: 'Switch to dark theme', keywords: 'appearance light', run: vi.fn() },
];

function press(target: Element, key: string) {
  act(() => { target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })); });
}

function Harness({ items }: { items: Command[] }) {
  const [open, setOpen] = useState(true);
  return <CommandPalette open={open} onClose={() => setOpen(false)} commands={items} />;
}

function render(ui: React.ReactNode) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root!.render(ui));
  return container!;
}

describe('filterCommands', () => {
  it('matches labels and keywords case-insensitively', () => {
    const hits = filterCommands(commands, 'RUFF');
    expect(hits.map((c) => c.id)).toEqual(['tools']);
  });

  it('ranks label prefixes above mid-string keyword matches', () => {
    const ranked = filterCommands([
      { id: 'infix', label: 'Quick scan workspace', run: () => {} },
      { id: 'prefix', label: 'Scan now', keywords: 'quick', run: () => {} },
    ], 'scan');
    expect(ranked.map((c) => c.id)).toEqual(['prefix', 'infix']);
  });
});

describe('CommandPalette interactions', () => {
  it('filters as you type and runs the highlighted command on Enter', () => {
    const dom = render(<Harness items={commands} />);
    const input = dom.querySelector<HTMLInputElement>('input')!;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
    act(() => {
      setter.call(input, 'too');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    const options = [...dom.querySelectorAll('[role="option"]')];
    expect(options).toHaveLength(1);
    expect(options[0].textContent).toContain('Go to Tools');
    press(input, 'Enter');
    expect((commands[1].run as ReturnType<typeof vi.fn>)).toHaveBeenCalledTimes(1);
  });

  it('moves the active option with arrow keys and announces via aria-activedescendant', () => {
    const dom = render(<Harness items={commands} />);
    const input = dom.querySelector<HTMLInputElement>('input')!;
    press(input, 'ArrowDown');
    press(input, 'ArrowDown');
    const options = [...dom.querySelectorAll('[role="option"]')];
    expect(options[2].getAttribute('aria-selected')).toBe('true');
    expect(input.getAttribute('aria-activedescendant')).toBe('command-option-theme');
    press(input, 'Enter');
    expect((commands[2].run as ReturnType<typeof vi.fn>)).toHaveBeenCalledTimes(1);
  });

  it('shows a live empty message when nothing matches', () => {
    const dom = render(<Harness items={commands} />);
    const input = dom.querySelector<HTMLInputElement>('input')!;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
    act(() => {
      setter.call(input, 'zzz');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(dom.querySelector('.palette-empty')?.textContent).toContain('No matching command');
  });
});
