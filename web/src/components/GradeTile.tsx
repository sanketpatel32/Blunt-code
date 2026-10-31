import { useEffect, useState } from 'react';
import { cn } from '../lib/utils';
import SplitFlapText from './kokonutui/split-flap-text';

export type GradeTileSize = 'sm' | 'md' | 'lg' | 'xl';

interface GradeTileProps {
  /** A–D, or null when nothing is graded yet. */
  grade: string | null | undefined;
  size?: GradeTileSize;
  /** Flip the letter in on a split-flap board (hero tiles only). */
  flip?: boolean;
  /** Text shown when there is no grade (default an en dash). */
  placeholder?: string;
  /** Tone override for non-grade glyphs such as ✓ or !. */
  tone?: 'clear' | 'warning';
  className?: string;
  style?: React.CSSProperties;
  title?: string;
  /** Read the letter to assistive tech (table cells with no other text). */
  announce?: boolean;
}

const FLIPPABLE = /^[A-Z!]$/;

/**
 * The signature object: a chunky letter-grade tile, one hue per grade on every
 * page (A teal, B sun, C orange, D coral-red). Hero tiles flip the letter in
 * on a Kokonut split-flap board; reduced motion shows it instantly.
 * Decorative by default — the caller owns the accessible text — unless
 * `announce` is set, where the (static) letter itself is the text.
 */
export function GradeTile({ grade, size = 'md', flip = false, placeholder = '–', tone, className, style, title, announce = false }: GradeTileProps) {
  const letter = grade ?? placeholder;
  const canFlip = flip && FLIPPABLE.test(letter);
  // Start blank and set the letter after mount so the board visibly turns.
  const [shown, setShown] = useState(canFlip ? ' ' : letter);
  useEffect(() => {
    if (!canFlip) { setShown(letter); return; }
    const id = window.setTimeout(() => setShown(letter), 120);
    return () => window.clearTimeout(id);
  }, [letter, canFlip]);

  return (
    <span
      className={cn('grade-tile', `grade-tile-${size}`, className)}
      data-grade={grade ?? 'none'}
      data-tone={tone}
      style={style}
      title={title}
      aria-hidden={announce && !canFlip ? undefined : true}
    >
      {canFlip
        ? <SplitFlapText text={shown} length={1} className="grade-flap" tileClassName="grade-flap-tile" />
        : <span className="grade-tile-letter">{letter}</span>}
    </span>
  );
}
