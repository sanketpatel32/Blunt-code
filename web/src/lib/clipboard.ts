/**
 * Copies plain text, preferring the async Clipboard API and falling back to a
 * temporary off-screen textarea + execCommand for non-secure contexts — the
 * local server serves plain http on loopback, where navigator.clipboard is
 * undefined and only the legacy path works. Resolves false when both fail so
 * callers can skip the success feedback.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // A rejected clipboard promise falls through to the textarea fallback.
  }
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.append(area);
  area.select();
  let copied = false;
  try {
    copied = document.execCommand('copy');
  } catch {
    copied = false;
  }
  area.remove();
  return copied;
}
