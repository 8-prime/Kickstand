export function Stat({
  k,
  v,
  u,
  tone = "ink",
}: {
  k: string;
  v: string | number;
  u?: string;
  tone?: "ink" | "ride" | "transfer";
}) {
  const color =
    tone === "ride" ? "text-ride" : tone === "transfer" ? "text-transfer" : "text-ink";
  return (
    <div>
      <div className="eyebrow">{k}</div>
      <div className={`font-data text-2xl leading-tight font-bold tracking-tight ${color}`}>
        {v}
        {u && <span className="ml-1 text-xs font-normal text-ink-soft">{u}</span>}
      </div>
    </div>
  );
}

export function Figure({ k, v }: { k: string; v: string }) {
  return (
    <div>
      <span className="block font-data text-[9px] tracking-[0.14em] text-ink-soft uppercase">
        {k}
      </span>
      <span className="font-data text-[15px] font-bold">{v}</span>
    </div>
  );
}
