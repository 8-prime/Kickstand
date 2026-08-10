import { useEffect, useRef, useState } from "react";
import { useTripStore } from "../store/useTripStore";

interface Props {
  /** Where this value lives in the document: `campsites[0].phone`. */
  path: string;
  value: string | number | undefined;
  /** Rendered when there is no value, and used as the input placeholder. */
  placeholder?: string;
  kind?: "text" | "multiline" | "number";
  className?: string;
  /** Shown to a reader who cannot edit, when the value is empty. */
  emptyLabel?: string;
}

/** Click a value, change it, and it saves.
 *
 *  Read-only visitors see plain text — the edit affordance never appears for
 *  someone holding a view link, rather than appearing and then failing. */
export function Editable({
  path,
  value,
  placeholder,
  kind = "text",
  className = "",
  emptyLabel = "—",
}: Props) {
  const canEdit = useTripStore((s) => s.payload?.access === "edit");
  const patch = useTripStore((s) => s.patch);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const ref = useRef<HTMLInputElement & HTMLTextAreaElement>(null);

  useEffect(() => {
    if (editing) ref.current?.focus();
  }, [editing]);

  if (!canEdit) {
    const text = value === undefined || value === "" ? emptyLabel : String(value);
    return <span className={className}>{text}</span>;
  }

  if (!editing) {
    const empty = value === undefined || value === "";
    return (
      <button
        type="button"
        onClick={() => {
          setDraft(value === undefined ? "" : String(value));
          setEditing(true);
        }}
        title="Click to edit"
        className={[
          "cursor-text text-left underline decoration-paper-edge decoration-dashed underline-offset-4 hover:decoration-ink",
          empty ? "text-ink-soft italic" : "",
          className,
        ].join(" ")}
      >
        {empty ? (placeholder ?? emptyLabel) : String(value)}
      </button>
    );
  }

  const commit = async () => {
    setEditing(false);
    const next = kind === "number" ? Number(draft) : draft;
    if (next === value || (kind === "number" && Number.isNaN(next))) return;
    setSaving(true);
    await patch(path, next);
    setSaving(false);
  };

  const shared = {
    ref,
    value: draft,
    disabled: saving,
    placeholder,
    onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setDraft(e.target.value),
    onBlur: commit,
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        setEditing(false);
      }
      // Enter saves, except in a multiline field where it is a newline.
      if (e.key === "Enter" && kind !== "multiline") {
        e.preventDefault();
        void commit();
      }
    },
    className: `field w-full ${className}`,
  };

  return kind === "multiline" ? (
    <textarea {...shared} rows={3} />
  ) : (
    <input {...shared} type={kind === "number" ? "number" : "text"} />
  );
}
