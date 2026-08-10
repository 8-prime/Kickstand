import { useState } from "react";
import type { FieldError, TripDoc, TripPayload } from "../types";
import { ApiError, api, adminToken } from "../api/client";
import { Editable } from "../components/Editable";
import { useTripStore } from "../store/useTripStore";

/** The trip document itself: get it out, put a new one in, and hand out links.
 *
 *  This is the bulk-edit path. Everything here works on the whole document, so
 *  it is also the answer to "have a model write me a trip" — point it at the
 *  schema, paste the result. */
export function TripDataView({ doc, payload }: { doc: TripDoc; payload: TripPayload }) {
  const canEdit = payload.access === "edit";

  return (
    <div className="space-y-4">
      <Meta doc={doc} />
      {canEdit && <ImportPanel payload={payload} />}
      <SharePanel payload={payload} />
      <SchemaPanel />
    </div>
  );
}

function Meta({ doc }: { doc: TripDoc }) {
  return (
    <section className="panel p-4">
      <p className="eyebrow">This trip</p>
      <h2 className="mt-1.5 text-2xl">
        <Editable path="name" value={doc.name} />
      </h2>
      <p className="mt-1 text-[14.5px] text-ink-soft">
        <Editable path="subtitle" value={doc.subtitle} placeholder="A subtitle" />
      </p>
      <div className="mt-3 flex flex-wrap gap-x-8 gap-y-3 font-data text-[13px]">
        <Field label="From">
          <Editable path="origin" value={doc.origin} placeholder="Where from?" />
        </Field>
        <Field label="Dates">
          <Editable path="dates" value={doc.dates} placeholder="When?" />
        </Field>
        <Field label="Van out">
          <Editable path="vanIn" value={doc.vanIn} kind="number" className="w-20" /> km
        </Field>
        <Field label="Van home">
          <Editable path="vanOut" value={doc.vanOut} kind="number" className="w-20" /> km
        </Field>
      </div>
    </section>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <span className="block font-data text-[9px] tracking-[0.14em] text-ink-soft uppercase">
        {label}
      </span>
      <span className="font-data text-[15px] font-bold">{children}</span>
    </div>
  );
}

function ImportPanel({ payload }: { payload: TripPayload }) {
  const replaceDoc = useTripStore((s) => s.replaceDoc);
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<FieldError[]>([]);
  const [warnings, setWarnings] = useState<FieldError[]>([]);
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (raw: string) => {
    setErrors([]);
    setWarnings([]);
    setMessage(null);

    let parsed: TripDoc;
    try {
      parsed = JSON.parse(raw);
    } catch (err) {
      setErrors([
        {
          path: "",
          message: err instanceof Error ? err.message : "That is not valid JSON.",
        },
      ]);
      return;
    }

    setBusy(true);
    try {
      await replaceDoc(parsed);
      setText("");
      setMessage("Imported. The map, roadbook and checklist now use the new document.");
    } catch (err) {
      if (err instanceof ApiError) {
        setMessage(err.message);
        setErrors(err.errors);
        setWarnings(err.warnings);
      } else {
        setMessage("Could not import that.");
      }
    } finally {
      setBusy(false);
    }
  };

  const onFile = async (file: File) => submit(await file.text());

  return (
    <section className="panel p-4">
      <p className="eyebrow">Replace the trip document</p>
      <p className="mt-1.5 text-[14.5px] text-ink-soft">
        Paste or upload a whole trip. Days, stops, campsites and the checklist all come from
        here. Your ride log, ticked items and cached routes survive the swap — they are keyed
        by day number and checklist id, so keep those stable if you want to keep them.
      </p>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <a
          href={api.exportUrl(payload.editToken ?? payload.viewToken ?? "")}
          className="border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper"
          download
        >
          Download this trip
        </a>
        <label className="cursor-pointer border border-paper-edge px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:border-ink">
          Upload a file
          <input
            type="file"
            accept="application/json,.json"
            className="sr-only"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void onFile(f);
              e.target.value = "";
            }}
          />
        </label>
      </div>

      <textarea
        rows={8}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="…or paste a trip document here"
        className="field mt-3 w-full resize-y font-data text-[12px] leading-relaxed"
        spellCheck={false}
      />
      <button
        type="button"
        disabled={busy || !text.trim()}
        onClick={() => void submit(text)}
        className="mt-2 border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper disabled:opacity-40"
      >
        {busy ? "Checking…" : "Import what is pasted"}
      </button>

      {message && (
        <p
          className={`mt-3 text-[13.5px] ${errors.length ? "text-alert" : "text-transfer"}`}
        >
          {message}
        </p>
      )}
      <Problems title="Fix these" items={errors} tone="alert" />
      <Problems title="Worth a look" items={warnings} tone="soft" />
    </section>
  );
}

function Problems({
  title,
  items,
  tone,
}: {
  title: string;
  items: FieldError[];
  tone: "alert" | "soft";
}) {
  if (!items.length) return null;
  return (
    <div className="mt-3">
      <p className={`eyebrow ${tone === "alert" ? "text-alert" : ""}`}>{title}</p>
      <ul className="mt-1.5 space-y-1">
        {items.map((e, i) => (
          <li key={i} className="font-data text-[11.5px] leading-relaxed">
            <span className={tone === "alert" ? "text-alert" : "text-ink"}>
              {e.path || "document"}
            </span>
            <span className="text-ink-soft"> — {e.message}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function SharePanel({ payload }: { payload: TripPayload }) {
  const [tokens, setTokens] = useState({
    view: payload.viewToken,
    edit: payload.editToken,
  });
  const [busy, setBusy] = useState(false);

  if (!tokens.view && !tokens.edit) {
    return (
      <section className="panel p-4">
        <p className="eyebrow">Sharing</p>
        <p className="mt-1.5 text-[14.5px] text-ink-soft">
          Share links are shown to whoever holds the server's admin token. Add it on the trips
          page to manage links from here.
        </p>
      </section>
    );
  }

  const link = (token?: string) =>
    token ? `${window.location.origin}/t/${token}` : "";

  const rotate = async () => {
    if (
      !window.confirm(
        "Issue new links? Everyone using the current ones loses access immediately.",
      )
    )
      return;
    setBusy(true);
    try {
      const next = await api.rotateTokens(tokens.edit ?? tokens.view!);
      setTokens({ view: next.viewToken, edit: next.editToken });
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="panel p-4">
      <p className="eyebrow">Sharing</p>
      <p className="mt-1.5 text-[14.5px] text-ink-soft">
        Anyone with a link is in — there is no sign-in. Hand out the read-only one unless you
        want someone logging distances and editing the plan.
      </p>

      <div className="mt-3 space-y-2">
        <LinkRow label="Read only" url={link(tokens.view)} />
        <LinkRow label="Can edit" url={link(tokens.edit)} />
      </div>

      <button
        type="button"
        disabled={busy}
        onClick={() => void rotate()}
        className="mt-3 border border-paper-edge px-3 py-1.5 font-data text-[10px] tracking-[0.12em] text-ink-soft uppercase transition-colors hover:border-alert hover:text-alert disabled:opacity-50"
      >
        {busy ? "Issuing…" : "Issue new links"}
      </button>
    </section>
  );
}

function LinkRow({ label, url }: { label: string; url: string }) {
  const [copied, setCopied] = useState(false);
  if (!url) return null;

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="w-24 font-data text-[9px] tracking-[0.14em] text-ink-soft uppercase">
        {label}
      </span>
      <input readOnly value={url} className="field min-w-0 flex-1 text-[11px]" />
      <button
        type="button"
        onClick={() => {
          void navigator.clipboard.writeText(url).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          });
        }}
        className="border border-paper-edge px-2 py-1.5 font-data text-[10px] tracking-[0.1em] uppercase hover:border-ink"
      >
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  );
}

function SchemaPanel() {
  const hasAdmin = !!adminToken();
  return (
    <section className="panel p-4">
      <p className="eyebrow">Writing a trip from scratch</p>
      <p className="mt-1.5 text-[14.5px] text-ink-soft">
        The document format is published as a JSON Schema. Give a model the schema and a
        worked example and ask for a trip in the same shape, then paste the result above.
        Anything wrong comes back as a list of fields to fix rather than a failed import.
      </p>
      <div className="mt-3 flex flex-wrap gap-4 font-data text-[11px]">
        <a href={api.schemaUrl()} target="_blank" rel="noreferrer" className="underline underline-offset-2">
          trip.json — the schema
        </a>
        <a href={api.exampleUrl()} target="_blank" rel="noreferrer" className="underline underline-offset-2">
          example.json — a complete trip
        </a>
        {hasAdmin && (
          <a href="/" className="underline underline-offset-2">
            all trips on this server
          </a>
        )}
      </div>
    </section>
  );
}
