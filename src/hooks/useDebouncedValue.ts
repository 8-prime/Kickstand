import { useEffect, useRef, useState } from "react";

/** A text field that types locally and saves a beat later.
 *
 *  Without this, every keystroke in a note is an HTTP request. The draft is
 *  what you see while typing; `commit` runs once you pause, and again on
 *  unmount so a half-typed note is not lost by navigating away.
 *
 *  When the server value changes underneath you — someone else edited it —
 *  the draft is replaced, but only while you are not mid-edit. */
export function useDebouncedValue<T extends string | number | null>(
  value: T,
  commit: (next: T) => void,
  delay = 500,
) {
  const [draft, setDraft] = useState<T>(value);
  const dirty = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const latest = useRef({ draft, commit });
  latest.current = { draft, commit };

  useEffect(() => {
    if (!dirty.current) setDraft(value);
  }, [value]);

  useEffect(() => {
    return () => {
      // Flush on unmount: switching view mid-note must not drop it.
      if (timer.current) clearTimeout(timer.current);
      if (dirty.current) latest.current.commit(latest.current.draft);
    };
  }, []);

  const change = (next: T) => {
    dirty.current = true;
    setDraft(next);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      dirty.current = false;
      commit(next);
    }, delay);
  };

  const flush = () => {
    if (timer.current) clearTimeout(timer.current);
    if (dirty.current) {
      dirty.current = false;
      commit(draft);
    }
  };

  return { draft, change, flush };
}
