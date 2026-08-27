import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";

export type GraphNode = {
  id: string;
  label: string;
  kind: "workspace" | "file" | "package";
  language?: string;
  findingCount?: number;
};

export type GraphEdge = { from: string; to: string };

const VIEW_W = 800;
const VIEW_H = 360;
const PADDING = 40;

/** Derive a deterministic mock graph from workspace languages. */
export function mockGraphFromLanguages(languages?: string[]): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const langs = languages?.length ? languages.slice(0, 6) : ["typescript", "python"];
  const nodes: GraphNode[] = [
    { id: "workspace", label: "workspace", kind: "workspace", findingCount: langs.length * 2 },
  ];
  const edges: GraphEdge[] = [];
  for (const lang of langs) {
    const fileId = `file:${lang}`;
    nodes.push({
      id: fileId,
      label: `${lang}`,
      kind: "file",
      language: lang,
      findingCount: Math.floor(Math.random() * 4),
    });
    edges.push({ from: "workspace", to: fileId });
    // add one dependent package node per language for richer graph
    const pkgId = `pkg:${lang}`;
    nodes.push({ id: pkgId, label: `${lang}-deps`, kind: "package", findingCount: Math.floor(Math.random() * 2) });
    edges.push({ from: fileId, to: pkgId });
  }
  // cross edge between first two files if multiple
  if (langs.length > 1) edges.push({ from: `file:${langs[0]}`, to: `file:${langs[1]}` });
  return { nodes, edges };
}

type Pos = { x: number; y: number; vx: number; vy: number };

function initialPositions(nodes: GraphNode[]): Record<string, Pos> {
  const cx = VIEW_W / 2;
  const cy = VIEW_H / 2;
  const r = Math.min(VIEW_W, VIEW_H) / 2 - PADDING;
  const map: Record<string, Pos> = {};
  nodes.forEach((n, i) => {
    if (n.kind === "workspace") {
      map[n.id] = { x: cx, y: cy, vx: 0, vy: 0 };
    } else {
      const angle = (i / Math.max(1, nodes.length - 1)) * Math.PI * 2 - Math.PI / 2;
      const rad = n.kind === "file" ? r * 0.68 : r;
      map[n.id] = { x: cx + Math.cos(angle) * rad, y: cy + Math.sin(angle) * rad, vx: 0, vy: 0 };
    }
  });
  return map;
}

function nodeRadius(node: GraphNode): number {
  if (node.kind === "workspace") return 18;
  if (node.kind === "file") return 13;
  return 9;
}

function nodeFill(node: GraphNode): string {
  if (node.kind === "workspace") return "var(--color-accent)";
  if (node.kind === "file") return "var(--color-accent-strong)";
  return "var(--color-ink-faint)";
}

export function DependencyGraph({
  nodes: propNodes,
  edges: propEdges,
  languages,
}: {
  nodes?: GraphNode[];
  edges?: GraphEdge[];
  languages?: string[];
}) {
  const { nodes, edges } = useMemo(() => {
    if (propNodes?.length) return { nodes: propNodes, edges: propEdges ?? [] };
    return mockGraphFromLanguages(languages);
  }, [propNodes, propEdges, languages]);

  const [positions, setPositions] = useState<Record<string, Pos>>(() => initialPositions(nodes));
  const [hovered, setHovered] = useState<string | null>(null);
  const [scale, setScale] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [dragging, setDragging] = useState<string | null>(null);
  const [panning, setPanning] = useState(false);
  const panStart = useRef({ x: 0, y: 0, ox: 0, oy: 0 });
  const svgRef = useRef<SVGSVGElement>(null);

  // reset when nodes change
  useEffect(() => {
    setPositions(initialPositions(nodes));
  }, [nodes]);

  // simple force simulation ticks (reduced-motion disables)
  useEffect(() => {
    const media = window.matchMedia?.("(prefers-reduced-motion: reduce)");
    if (media?.matches) return;
    let raf = 0;
    let ticks = 0;
    const maxTicks = 90;
    const tick = () => {
      if (ticks++ > maxTicks) return;
      setPositions((prev) => {
        const next: Record<string, Pos> = { ...prev };
        // copy
        for (const k of Object.keys(next)) next[k] = { ...next[k] };
        const ids = nodes.map((n) => n.id);
        // repulsion
        for (let i = 0; i < ids.length; i++) {
          for (let j = i + 1; j < ids.length; j++) {
            const a = next[ids[i]];
            const b = next[ids[j]];
            const dx = a.x - b.x;
            const dy = a.y - b.y;
            const dist = Math.hypot(dx, dy) || 1;
            const minDist = 70;
            if (dist < minDist * 2) {
              const force = (minDist / dist) * 0.25;
              const fx = (dx / dist) * force;
              const fy = (dy / dist) * force;
              if (nodes.find((n) => n.id === ids[i])?.kind !== "workspace") { a.x += fx; a.y += fy; }
              if (nodes.find((n) => n.id === ids[j])?.kind !== "workspace") { b.x -= fx; b.y -= fy; }
            }
          }
        }
        // attraction along edges
        for (const e of edges) {
          const a = next[e.from];
          const b = next[e.to];
          if (!a || !b) continue;
          const dx = b.x - a.x;
          const dy = b.y - a.y;
          const dist = Math.hypot(dx, dy) || 1;
          const target = 110;
          const f = (dist - target) * 0.02;
          const fx = (dx / dist) * f;
          const fy = (dy / dist) * f;
          const aIsWs = nodes.find((n) => n.id === e.from)?.kind === "workspace";
          const bIsWs = nodes.find((n) => n.id === e.to)?.kind === "workspace";
          if (!aIsWs) { a.x += fx; a.y += fy; }
          if (!bIsWs) { b.x -= fx; b.y -= fy; }
        }
        // center gravity + clamp
        for (const id of ids) {
          const p = next[id];
          const isWs = nodes.find((n) => n.id === id)?.kind === "workspace";
          if (!isWs) {
            const cx = VIEW_W / 2;
            const cy = VIEW_H / 2;
            p.vx = (p.vx + (cx - p.x) * 0.005) * 0.88;
            p.vy = (p.vy + (cy - p.y) * 0.005) * 0.88;
            p.x += p.vx;
            p.y += p.vy;
          }
          const rad = nodeRadius(nodes.find((n) => n.id === id)!);
          p.x = Math.max(rad + 8, Math.min(VIEW_W - rad - 8, p.x));
          p.y = Math.max(rad + 8, Math.min(VIEW_H - rad - 8, p.y));
        }
        return next;
      });
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [nodes, edges]);

  const onNodePointerDown = useCallback((id: string, e: React.PointerEvent) => {
    (e.target as Element).setPointerCapture(e.pointerId);
    setDragging(id);
  }, []);

  const onPointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (dragging) {
        const rect = svgRef.current?.getBoundingClientRect();
        if (!rect) return;
        const sx = VIEW_W / rect.width;
        const sy = VIEW_H / rect.height;
        const x = (e.clientX - rect.left) * sx;
        const y = (e.clientY - rect.top) * sy;
        // invert pan/scale
        const ix = (x - pan.x) / scale;
        const iy = (y - pan.y) / scale;
        setPositions((prev) => ({ ...prev, [dragging]: { ...prev[dragging], x: ix, y: iy, vx: 0, vy: 0 } }));
        return;
      }
      if (panning) {
        const dx = e.clientX - panStart.current.x;
        const dy = e.clientY - panStart.current.y;
        setPan({ x: panStart.current.ox + dx * (VIEW_W / (svgRef.current?.getBoundingClientRect().width ?? VIEW_W)), y: panStart.current.oy + dy * (VIEW_H / (svgRef.current?.getBoundingClientRect().height ?? VIEW_H)) });
      }
    },
    [dragging, panning, pan.x, pan.y, scale],
  );

  const onPointerUp = useCallback(() => {
    setDragging(null);
    setPanning(false);
  }, []);

  const onBackgroundPointerDown = useCallback((e: React.PointerEvent) => {
    // only pan when clicking background, not nodes (nodes stop propagation)
    setPanning(true);
    panStart.current = { x: e.clientX, y: e.clientY, ox: pan.x, oy: pan.y };
  }, [pan.x, pan.y]);

  const zoomIn = useCallback(() => setScale((s) => Math.min(2.5, +(s + 0.2).toFixed(2))), []);
  const zoomOut = useCallback(() => setScale((s) => Math.max(0.5, +(s - 0.2).toFixed(2))), []);
  const resetView = useCallback(() => { setScale(1); setPan({ x: 0, y: 0 }); }, []);

  const hoveredNode = hovered ? nodes.find((n) => n.id === hovered) : null;
  const hoveredPos = hovered ? positions[hovered] : null;

  return (
    <Card className="rounded-[var(--radius-card)] border border-[var(--color-rule)] bg-[var(--color-surface)] shadow-[var(--shadow-card)]">
      <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
        <CardTitle className="text-sm font-semibold">Dependency graph</CardTitle>
        <div className="flex items-center gap-1" role="toolbar" aria-label="Graph controls">
          <button type="button" className="button secondary" style={{ minHeight: "1.9rem", padding: "0 var(--space-sm)" }} onClick={zoomIn} aria-label="Zoom in">+</button>
          <button type="button" className="button secondary" style={{ minHeight: "1.9rem", padding: "0 var(--space-sm)" }} onClick={zoomOut} aria-label="Zoom out">−</button>
          <button type="button" className="button secondary" style={{ minHeight: "1.9rem", padding: "0 var(--space-sm)" }} onClick={resetView} aria-label="Reset view">Reset</button>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="relative overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)]" style={{ touchAction: "none" }}>
          <svg
            ref={svgRef}
            viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
            role="img"
            aria-label={`Dependency graph with ${nodes.length} nodes and ${edges.length} connections. Drag nodes to reposition, drag background to pan.`}
            className="block h-[22rem] w-full select-none"
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
            onPointerLeave={onPointerUp}
            onPointerDown={onBackgroundPointerDown}
          >
            <g transform={`translate(${pan.x} ${pan.y}) scale(${scale})`}>
              {edges.map((e, i) => {
                const a = positions[e.from];
                const b = positions[e.to];
                if (!a || !b) return null;
                return <line key={`${e.from}-${e.to}-${i}`} x1={a.x} y1={a.y} x2={b.x} y2={b.y} stroke="var(--color-rule-strong)" strokeWidth={1.4} strokeOpacity={0.7} />;
              })}
              {nodes.map((n) => {
                const p = positions[n.id];
                if (!p) return null;
                const r = nodeRadius(n);
                const isHovered = hovered === n.id;
                return (
                  <g
                    key={n.id}
                    tabIndex={0}
                    role="button"
                    aria-label={`${n.label} ${n.kind}${n.findingCount !== undefined ? `, ${n.findingCount} findings` : ""}`}
                    onPointerDown={(e) => { e.stopPropagation(); onNodePointerDown(n.id, e); }}
                    onPointerEnter={() => setHovered(n.id)}
                    onPointerLeave={() => setHovered((cur) => (cur === n.id ? null : cur))}
                    onFocus={() => setHovered(n.id)}
                    onBlur={() => setHovered((cur) => (cur === n.id ? null : cur))}
                    style={{ cursor: dragging === n.id ? "grabbing" : "grab", outline: "none" }}
                  >
                    <circle cx={p.x} cy={p.y} r={r + (isHovered ? 3 : 0)} fill="var(--color-surface)" opacity={0} />
                    <circle
                      cx={p.x}
                      cy={p.y}
                      r={r}
                      fill={nodeFill(n)}
                      stroke="var(--color-surface)"
                      strokeWidth={2}
                      style={{ filter: isHovered ? "brightness(1.08)" : undefined, transition: "transform 140ms var(--ease-out)", transformOrigin: `${p.x}px ${p.y}px` }}
                    />
                    {n.kind === "workspace" && <text x={p.x} y={p.y + 4} textAnchor="middle" fill="white" fontSize={9} fontWeight={700} pointerEvents="none" aria-hidden="true">WS</text>}
                    <text x={p.x} y={p.y + r + 13} textAnchor="middle" fill="var(--color-ink-soft)" fontSize={10} fontWeight={600} fontFamily="var(--font-mono)" pointerEvents="none">
                      {n.label}
                    </text>
                  </g>
                );
              })}
            </g>
          </svg>
          {hoveredNode && hoveredPos && (
            <div
              role="tooltip"
              className="pointer-events-none absolute z-10 rounded-[var(--radius-md)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 py-2 text-xs shadow-[var(--shadow-lg)]"
              style={{
                left: `calc(${(hoveredPos.x / VIEW_W) * 100}% + ${pan.x}px)`,
                top: `calc(${(hoveredPos.y / VIEW_H) * 100}% + ${pan.y}px)`,
                transform: "translate(-50%, calc(-100% - 14px))",
              }}
            >
              <strong className="block text-[var(--color-ink)]">{hoveredNode.label}</strong>
              <span className="text-[var(--color-ink-soft)]">
                {hoveredNode.kind} {hoveredNode.language ? `· ${hoveredNode.language}` : ""} · {hoveredNode.findingCount ?? 0} {hoveredNode.findingCount === 1 ? "finding" : "findings"}
              </span>
            </div>
          )}
        </div>
        <p className="mt-2 text-xs text-[var(--color-ink-faint)]">Drag nodes to reposition · drag background to pan · use zoom controls · hover or focus a node for its finding count.</p>
      </CardContent>
    </Card>
  );
}
