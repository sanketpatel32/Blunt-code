import { useEffect, useState } from 'react';

export function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  });

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return;
    const m = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = () => setReduced(m.matches);
    if (typeof m.addEventListener === 'function') m.addEventListener('change', onChange);
    else if (typeof m.addListener === 'function') m.addListener(onChange);
    return () => {
      if (typeof m.removeEventListener === 'function') m.removeEventListener('change', onChange);
      else if (typeof m.removeListener === 'function') m.removeListener(onChange);
    };
  }, []);

  return reduced;
}
