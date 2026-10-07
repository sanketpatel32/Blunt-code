import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ScanReviewDialog } from './ScanReviewDialog';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
afterEach(async () => { await act(async () => root?.unmount()); document.body.replaceChildren(); vi.unstubAllGlobals(); });
const planned = { selected_files: 3, candidate_files: 5, dependency_inputs: ['go.mod'], exclusions: ['vendor/**'], skip_counts: { excluded_user: 2 }, analyzers: [{ id: 'osv-dependencies', display_name: 'OSV Scanner', planned: true, ready: false, registered: true, reason: 'Setup required', network: 'outbound', network_note: 'Queries advisory services.' }] };
async function render(body: unknown, status = 200) {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })));
  const host = document.createElement('div'); document.body.append(host); root = createRoot(host);
  const confirm = vi.fn();
  await act(async () => { root.render(<ScanReviewDialog workspaceId="ws-1" workspaceName="Example" profile="deep" confirmLabel="Run deep scan" busy={false} onCancel={() => {}} onConfirm={confirm} />); });
  for (let i = 0; i < 3; i++) await act(async () => { await Promise.resolve(); });
  const dialog = document.querySelector('[role="dialog"]')!;
  const button = [...dialog.querySelectorAll('button')].find((b) => b.textContent === 'Run deep scan')!;
  return { dialog, button, confirm };
}
describe('authoritative scan review', () => {
  it('shows real selection and outbound network before explicitly starting', async () => {
    const { dialog, button, confirm } = await render(planned);
    expect(dialog.textContent).toContain('3 selected source files');
    expect(dialog.textContent).toContain('May contact external services');
    expect(dialog.textContent).toContain('Setup required');
    expect(confirm).not.toHaveBeenCalled();
    expect(button.disabled).toBe(false);
    await act(async () => button.click());
    expect(confirm).toHaveBeenCalledOnce();
  });
  it('blocks starting when no analyzers have applicable inputs', async () => {
    const { dialog, button } = await render({ ...planned, analyzers: [] });
    expect(button.disabled).toBe(true);
    expect(dialog.textContent).toContain('No analyzers have applicable inputs');
  });
  it('blocks starting and offers retry when discovery fails', async () => {
    const { dialog, button } = await render({ error: { code: 'DISCOVERY_FAILED', message: 'Folder is unavailable.' } }, 500);
    expect(button.disabled).toBe(true);
    expect(dialog.textContent).toContain('Folder is unavailable.');
    expect(dialog.textContent).toContain('Retry review');
  });
  it('does not treat an invalid response as an empty or ready plan', async () => {
    const { dialog, button } = await render({});
    expect(button.disabled).toBe(true);
    expect(dialog.textContent).toContain('server did not return a scan plan');
  });
});
