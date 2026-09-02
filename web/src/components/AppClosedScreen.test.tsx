import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AppClosedScreen } from './AppClosedScreen';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

async function render(props: { mode: 'stopped' | 'updating'; version?: string }) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<AppClosedScreen mode={props.mode} version={props.version} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('AppClosedScreen', () => {
  it('reassures after a plain stop: data stays local, tab can be closed', async () => {
    const host = await render({ mode: 'stopped' });
    expect(host.textContent).toContain('Blunt Code has stopped');
    expect(host.textContent).toContain('stay saved on this computer');
    expect(host.textContent).toContain('Start Blunt Code again from the Start menu');
    expect(host.textContent).toContain('Close this tab');
    // The update stepper belongs to the updating mode only.
    expect(host.querySelector('.farewell-steps')).toBeNull();
  });

  it('walks the update steps and names the version being installed', async () => {
    const host = await render({ mode: 'updating', version: '0.17.0' });
    expect(host.textContent).toContain('Updating Blunt Code');
    expect(host.textContent).toContain('Version 0.17.0 is installing.');
    expect(host.textContent).toContain('opens in a new browser tab');
    expect([...host.querySelectorAll('.farewell-step')].map((step) => step.textContent)).toEqual([
      'Close the running app',
      'Download and install the new version',
      'Reopen Blunt Code automatically',
    ]);
  });

  it('explains when the browser refuses to close the tab', async () => {
    const close = vi.fn();
    vi.stubGlobal('close', close);
    const host = await render({ mode: 'stopped' });
    const button = [...host.querySelectorAll('button')].find((candidate) => candidate.textContent === 'Close this tab')!;
    await act(async () => { button.click(); });
    expect(close).toHaveBeenCalled();
    // A closed tab never runs timers — this one firing means the tab survived
    // and the fallback hint must appear.
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 450)); });
    expect(host.textContent).toContain('Your browser kept this tab open');
  });
});
