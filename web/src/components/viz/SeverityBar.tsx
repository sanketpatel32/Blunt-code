import { m, useReducedMotion } from 'motion/react';
import type { Severity } from '../../types';
import { SEVERITY_COLOR, SEVERITY_ORDER } from '../../lib/chartData';

/** Stacked severity bar with compact count chips; replaces "N critical, N
 *  high…" sentences. Zero bands are skipped in the bar but kept in chips when
 *  `showZero` is set, so a clean scan still reads as a full legend. */
export function SeverityBar({ counts, showZero = false, chips = true, height = 10, onSelect }: { counts: Partial<Record<Severity, number>>; showZero?: boolean; chips?: boolean; height?: number; onSelect?: (sev: Severity) => void }) {
  const reduced = useReducedMotion();
  const total = SEVERITY_ORDER.reduce((sum, k) => sum + (counts[k] ?? 0), 0);
  const label = `Severity: ${SEVERITY_ORDER.map((k) => `${counts[k] ?? 0} ${k}`).join(', ')}`;
  return (
    <div className="viz-sevbar">
      <div className="viz-sevbar-track" role="img" aria-label={label} style={{ height }}>
        {total === 0 ? <span className="viz-sevbar-empty" /> : SEVERITY_ORDER.filter((k) => (counts[k] ?? 0) > 0).map((k, i) => (
          <m.span
            key={k}
            className="viz-sevbar-seg"
            style={{ background: SEVERITY_COLOR[k], flexGrow: counts[k] ?? 0 }}
            initial={reduced ? false : { scaleX: 0 }}
            animate={{ scaleX: 1 }}
            transition={{ duration: 0.5, delay: i * 0.06, ease: [0.22, 1, 0.36, 1] }}
            title={`${counts[k]} ${k}`}
          />
        ))}
      </div>
      {chips && (
        <ul className="viz-sevchips" aria-label="Severity counts">
          {SEVERITY_ORDER.filter((k) => showZero || (counts[k] ?? 0) > 0).map((k) => {
            const body = <><i style={{ background: SEVERITY_COLOR[k] }} aria-hidden="true" /><span className="viz-sevchip-name">{k}</span><b>{counts[k] ?? 0}</b></>;
            return <li key={k}>{onSelect ? <button type="button" className="viz-sevchip" onClick={() => onSelect(k)}>{body}</button> : <span className="viz-sevchip">{body}</span>}</li>;
          })}
        </ul>
      )}
    </div>
  );
}
