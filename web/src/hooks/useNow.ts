import { useState, useEffect } from 'react';

/**
 * Returns the current timestamp (Date.now()) updated every second.
 * Use this in components that need real-time countdowns or relative time displays
 * without forcing parent re-renders.
 */
export function useNow(): number {
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  return now;
}
