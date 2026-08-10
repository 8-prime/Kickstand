/** Thin-space thousands, the way the roadbooks wrote them: 1 250 km.
 *  Grouped by hand rather than via toLocaleString, which inserts a narrow
 *  no-break space whose exact codepoint moves between ICU versions. */
export const km = (n: number): string =>
  String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, " ");

/** 5.5 → "5.5", 5 → "5". Hours read badly with a trailing zero. */
export const hrs = (n: number): string => String(Math.round(n * 10) / 10);

export const pad2 = (n: number): string => String(n).padStart(2, "0");

/** "Fri 11 Sep" → "FRI 11 SEP" */
export const stamp = (s: string): string => s.toUpperCase();
