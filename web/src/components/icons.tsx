import type { ReactNode, SVGProps } from 'react';

/**
 * Friendly line icons for empty states. All icons share the app's minimal stroke
 * style (24px grid, 1.75 stroke, rounded joins) and are decorative: they carry
 * aria-hidden and inherit color from the surrounding medallion.
 */
type IconProps = SVGProps<SVGSVGElement>;

function Svg({ children, ...props }: IconProps & { children: ReactNode }) {
  return <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false" {...props}>{children}</svg>;
}

/** No workspaces yet. */
export function FolderIcon(props: IconProps) {
  return <Svg {...props}><path d="M3.5 7.4c0-1 .85-1.9 1.9-1.9h3.1c.6 0 1.2.28 1.6.78l.86 1.06h7.64c1.05 0 1.9.85 1.9 1.9v7.36c0 1.05-.85 1.9-1.9 1.9H5.4a1.9 1.9 0 0 1-1.9-1.9V7.4z" /></Svg>;
}

/** No scans yet — a scanner viewfinder with a sweep line. */
export function ScanIcon(props: IconProps) {
  return <Svg {...props}><path d="M3.5 8V6.4A2.9 2.9 0 0 1 6.4 3.5H8" /><path d="M16 3.5h1.6a2.9 2.9 0 0 1 2.9 2.9V8" /><path d="M20.5 16v1.6a2.9 2.9 0 0 1-2.9 2.9H16" /><path d="M8 20.5H6.4a2.9 2.9 0 0 1-2.9-2.9V16" /><path d="M7.5 12h9" /></Svg>;
}

/** No matches — a plain magnifier. */
export function MagnifierIcon(props: IconProps) {
  return <Svg {...props}><circle cx="10.5" cy="10.5" r="6" /><path d="m15 15 5 5" /></Svg>;
}

/** All clear — a shield with a check. */
export function CheckShieldIcon(props: IconProps) {
  return <Svg {...props}><path d="M12 3.2 19 5.9v5.2c0 4.4-2.9 7.6-7 9.5-4.1-1.9-7-5.1-7-9.5V5.9L12 3.2z" /><path d="m8.9 12.1 2.1 2.1 4.1-4.3" /></Svg>;
}

/** No managed tools. */
export function WrenchIcon(props: IconProps) {
  return <Svg {...props}><path d="M14.7 6.3a1.1 1.1 0 0 0 0 1.5l1.5 1.5a1.1 1.1 0 0 0 1.5 0l3.7-3.7a5.9 5.9 0 0 1-7.9 7.9l-6.9 6.9a2.1 2.1 0 0 1-3-3l6.9-6.9a5.9 5.9 0 0 1 7.9-7.9l-3.7 3.7z" /></Svg>;
}

/** Celebration accents — a four-point sparkle. */
export function SparkleIcon(props: IconProps) {
  return <Svg {...props}><path d="M12 5.5c.65 3.3 2.7 5.35 6 6-3.3.65-5.35 2.7-6 6-.65-3.3-2.7-5.35-6-6 3.3-.65 5.35-2.7 6-6z" /></Svg>;
}
