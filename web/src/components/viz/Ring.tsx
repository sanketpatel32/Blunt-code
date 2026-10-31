import { m, useReducedMotion } from 'motion/react';

/** Progress ring (e.g. analyzers ready, coverage). Value and max are real
 *  counts; the centre shows "value/max" unless children override it. */
export function Ring({ value, max, size = 56, stroke = 6, tone = 'var(--color-accent)', label, children }: { value: number; max: number; size?: number; stroke?: number; tone?: string; label: string; children?: React.ReactNode }) {
  const reduced = useReducedMotion();
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const ratio = max > 0 ? Math.min(1, Math.max(0, value / max)) : 0;
  return (
    <span className="viz-ring" role="img" aria-label={label} style={{ width: size, height: size }}>
      <svg viewBox={`0 0 ${size} ${size}`} width={size} height={size} aria-hidden="true">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--color-rule)" strokeWidth={stroke} />
        <m.circle
          cx={size / 2} cy={size / 2} r={r} fill="none" stroke={tone} strokeWidth={stroke} strokeLinecap="round"
          strokeDasharray={c}
          initial={{ strokeDashoffset: reduced ? c * (1 - ratio) : c }}
          animate={{ strokeDashoffset: c * (1 - ratio) }}
          transition={{ duration: reduced ? 0 : 0.8, ease: [0.22, 1, 0.36, 1] }}
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
        />
      </svg>
      <span className="viz-ring-center" aria-hidden="true">{children ?? <><b>{value}</b><small>/{max}</small></>}</span>
    </span>
  );
}
