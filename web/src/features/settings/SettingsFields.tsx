import { useEffect, useState, type ReactNode } from "react";

export function SettingsSection(props: {
  id: string;
  eyebrow: string;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <section className="settings-section" id={props.id}>
      <header>
        <span className="eyebrow">{props.eyebrow}</span>
        <h2>{props.title}</h2>
        <p>{props.description}</p>
      </header>
      <div className="settings-card">{props.children}</div>
    </section>
  );
}

export function Field(props: { label: string; hint?: string; children: ReactNode; wide?: boolean }) {
  return (
    <label className={`settings-field ${props.wide ? "wide" : ""}`}>
      <span>{props.label}</span>
      {props.children}
      {props.hint && <small>{props.hint}</small>}
    </label>
  );
}

export function Toggle(props: { label: string; hint?: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="toggle-row">
      <span><strong>{props.label}</strong>{props.hint && <small>{props.hint}</small>}</span>
      <input type="checkbox" checked={props.checked} onChange={(event) => props.onChange(event.target.checked)} />
    </label>
  );
}

export function parseList(value: string): string[] {
  return value.split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
}

export function listText(value: string[] | undefined): string {
  return (value ?? []).join("\n");
}

export function JsonMapField(props: {
  label: string;
  value?: Record<string, string>;
  onChange: (value: Record<string, string>) => void;
}) {
  const serialized = JSON.stringify(props.value ?? {}, null, 2);
  const [draft, setDraft] = useState(serialized);
  const [error, setError] = useState("");
  useEffect(() => setDraft(serialized), [serialized]);
  const commit = () => {
    try {
      const parsed = JSON.parse(draft) as Record<string, unknown>;
      if (!parsed || Array.isArray(parsed) || Object.values(parsed).some((value) => typeof value !== "string")) throw new Error();
      props.onChange(parsed as Record<string, string>);
      setError("");
    } catch {
      setError("Use a JSON object with string values.");
    }
  };
  return (
    <Field label={props.label} hint={error || "JSON key/value map."}>
      <textarea rows={5} value={draft} onChange={(event) => setDraft(event.target.value)} onBlur={commit} aria-invalid={Boolean(error)} />
    </Field>
  );
}
