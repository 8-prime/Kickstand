import { useTripStore } from "../store/useTripStore";

/** Says, in one line, whether what you are looking at is current and whether
 *  what you typed has landed. Silent when everything is fine — a permanent
 *  green tick is noise. */
export function ConnectionStatus() {
  const online = useTripStore((s) => s.online);
  const pending = useTripStore((s) => s.pending);
  const fromCache = useTripStore((s) => s.fromCache);
  const cachedAt = useTripStore((s) => s.cachedAt);
  const error = useTripStore((s) => s.error);

  if (online && !pending && !fromCache && !error) return null;

  const parts: string[] = [];
  if (!online) parts.push("Offline");
  if (fromCache && cachedAt) parts.push(`showing the copy saved ${ago(cachedAt)}`);
  if (pending) parts.push(`${pending} change${pending === 1 ? "" : "s"} waiting to send`);

  return (
    <div
      role="status"
      className={[
        "mb-4 border-l-[3px] px-3 py-2 font-data text-[11px] leading-relaxed",
        error ? "border-l-alert text-alert" : "border-l-transfer text-ink-soft",
      ].join(" ")}
    >
      {parts.length > 0 && <span>{parts.join(" · ")}. </span>}
      {!online && pending > 0 && <span>They will go up when you have signal again. </span>}
      {error && <span>{error}</span>}
    </div>
  );
}

function ago(ts: number): string {
  const mins = Math.round((Date.now() - ts) / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} min ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} h ago`;
  return `${Math.round(hours / 24)} days ago`;
}
