/** Seven-day activity strip from /stats scans_by_day: one cell per local day,
 *  shade by scan count, a dot when any scan that day produced a result. */
export function HeatStrip({ days }: { days: Array<{ date: string; total: number; completed: number }> }) {
  if (!days.length) return null;
  const max = Math.max(...days.map((d) => d.total), 1);
  const fmt = (iso: string) => new Date(`${iso}T12:00:00`).toLocaleDateString(undefined, { weekday: 'narrow' });
  const label = `Scans in the last ${days.length} days: ${days.map((d) => `${d.date} ${d.total}`).join(', ')}`;
  return (
    <div className="viz-heat" role="img" aria-label={label}>
      {days.map((d) => (
        <span key={d.date} className="viz-heat-cell" title={`${d.date}: ${d.total} scan${d.total === 1 ? '' : 's'}`}>
          <span className="viz-heat-box" style={{ opacity: d.total ? 0.25 + 0.75 * (d.total / max) : 1 }} data-empty={d.total === 0 || undefined}>{d.total > 0 && <b>{d.total}</b>}</span>
          <small aria-hidden="true">{fmt(d.date)}</small>
        </span>
      ))}
    </div>
  );
}
