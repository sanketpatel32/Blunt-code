import { lazy, Suspense, type ComponentProps } from 'react';

/**
 * Lazy doors into the Recharts module. Importing from here never pulls
 * Recharts into the shell bundle: the first chart on a route fetches
 * chunk-recharts once, and a shimmer box of the same size holds its place.
 */
const load = () => import('./RechartsViz');
const LazyTrendArea = lazy(() => load().then((m) => ({ default: m.TrendArea })));
const LazySeverityDonut = lazy(() => load().then((m) => ({ default: m.SeverityDonut })));
const LazyGradeGauge = lazy(() => load().then((m) => ({ default: m.GradeGauge })));
const LazyCategoryBars = lazy(() => load().then((m) => ({ default: m.CategoryBars })));

export type { TrendDatum } from './RechartsViz';

function ChartSkeleton({ width, height, round }: { width?: number | string; height: number | string; round?: boolean }) {
  return <span className="viz-skeleton skeleton skeleton-anim" style={{ width: width ?? '100%', height, borderRadius: round ? '50%' : undefined }} aria-hidden="true" />;
}

type RV = typeof import('./RechartsViz');

export function TrendArea(props: ComponentProps<RV['TrendArea']>) {
  return <Suspense fallback={<ChartSkeleton height={props.height ?? 180} />}><LazyTrendArea {...props} /></Suspense>;
}
export function SeverityDonut(props: ComponentProps<RV['SeverityDonut']>) {
  const size = props.size ?? 148;
  return <Suspense fallback={<ChartSkeleton width={size} height={size} round />}><LazySeverityDonut {...props} /></Suspense>;
}
export function GradeGauge(props: ComponentProps<RV['GradeGauge']>) {
  const size = props.size ?? 132;
  return <Suspense fallback={<ChartSkeleton width={size} height={size} round />}><LazyGradeGauge {...props} /></Suspense>;
}
export function CategoryBars(props: ComponentProps<RV['CategoryBars']>) {
  return <Suspense fallback={<ChartSkeleton height={props.height ?? Math.max(72, Math.min(8, props.items.length) * 30 + 8)} />}><LazyCategoryBars {...props} /></Suspense>;
}
