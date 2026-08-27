type Props = {
  values?: number[];
  ariaLabel?: string;
};

export function MiniSparkline({ values, ariaLabel }: Props) {
  const data = values && values.length >= 2 ? values : undefined;
  if (!data) return null;
  const max = Math.max(...data, 1);
  const min = Math.min(...data);
  const range = Math.max(max - min, 1);
  const w = 64;
  const h = 20;
  const pad = 2;
  const step = (w - pad * 2) / (data.length - 1);
  const points = data.map((v, i) => {
    const x = pad + i * step;
    const y = h - pad - ((v - min) / range) * (h - pad * 2);
    return `${x},${y}`;
  });
  const poly = points.join(' ');
  const area = `${pad},${h - pad} ${poly} ${pad + (data.length - 1) * step},${h - pad}`;
  const label = ariaLabel ?? `Trend: ${data.join(', ')}`;
  return (
    <span className="inline-flex items-center align-middle ml-2" aria-label={label} role="img">
      <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} aria-hidden="true" className="overflow-visible">
        <polygon points={area} fill="var(--color-accent-soft)" opacity={0.9} />
        <polyline points={poly} fill="none" stroke="var(--color-accent)" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
        {data.length > 0 && (() => {
          const last = data[data.length - 1];
          const lx = pad + (data.length - 1) * step;
          const ly = h - pad - ((last - min) / range) * (h - pad * 2);
          return <circle cx={lx} cy={ly} r={2.2} fill="var(--color-accent)" />;
        })()}
      </svg>
    </span>
  );
}
