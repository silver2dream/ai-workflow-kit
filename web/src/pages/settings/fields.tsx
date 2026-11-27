import { useEffect, useId, useState, type ReactNode } from "react";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { decimalOf, EFFORT_WORDS, MODELS, numberOf } from "./model";

/** Field is a labelled control, with what it is for and what is wrong with it. */
export function Field({
  label,
  hint,
  error,
  className,
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  className?: string;
  children: (id: string, describedBy: string | undefined) => ReactNode;
}) {
  const id = useId();
  const hintID = hint ? `${id}-hint` : undefined;
  const errorID = error ? `${id}-error` : undefined;
  const describedBy = [hintID, errorID].filter(Boolean).join(" ") || undefined;
  return (
    <div className={cn("grid min-w-0 content-start gap-1", className)}>
      <label htmlFor={id} className="text-[13px] font-bold text-muted-foreground">
        {label}
      </label>
      {children(id, describedBy)}
      {hint && (
        <p id={hintID} className="text-[12.5px] leading-snug text-muted-foreground">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorID} className="text-[12.5px] font-bold text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}

const selectClass = "h-9 w-full min-w-0 rounded-lg border border-input bg-card px-2 text-[14px] outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50";

export function TextField({
  label,
  hint,
  error,
  value,
  onChange,
  placeholder,
  mono,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  mono?: boolean;
}) {
  return (
    <Field label={label} hint={hint} error={error}>
      {(id, describedBy) => (
        <Input
          id={id}
          value={value}
          placeholder={placeholder}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          onChange={(e) => onChange(e.target.value)}
          className={cn("h-9 text-[14px]", mono && "font-mono text-[13px]")}
        />
      )}
    </Field>
  );
}

/** NumberField holds a whole number, or nothing for the default it names. */
export function NumberField({
  label,
  hint,
  error,
  value,
  onChange,
  fallback,
  min = 1,
  max,
  unit,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  value: number | null;
  onChange: (value: number | null) => void;
  fallback: number;
  min?: number;
  max?: number;
  unit?: string;
}) {
  return (
    <Field label={label} hint={hint} error={error}>
      {(id, describedBy) => (
        <div className="flex items-center gap-2">
          <Input
            id={id}
            type="number"
            inputMode="numeric"
            min={min}
            max={max}
            step={1}
            value={value ?? ""}
            placeholder={`預設 ${fallback}`}
            aria-invalid={error ? true : undefined}
            aria-describedby={describedBy}
            onChange={(e) => onChange(numberOf(e.target.value))}
            className="h-9 w-32 text-[14px] tnum"
          />
          {unit && <span className="text-[13.5px] text-muted-foreground">{unit}</span>}
        </div>
      )}
    </Field>
  );
}

/**
 * MoneyField holds an amount in dollars, cents and all, or nothing for no
 * limit. What is typed is kept as typed ("1." stays while the cents come),
 * and taken again from the value when that changes from outside.
 */
export function MoneyField({
  label,
  hint,
  error,
  value,
  onChange,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  value: number | null;
  onChange: (value: number | null) => void;
}) {
  const [text, setText] = useState(value === null ? "" : String(value));
  useEffect(() => {
    if (decimalOf(text) !== value) setText(value === null ? "" : String(value));
    // Only a value from outside is taken again, not what the field just said.
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Field label={label} hint={hint} error={error}>
      {(id, describedBy) => (
        <div className="flex items-center gap-2">
          <span className="text-[13.5px] text-muted-foreground">US$</span>
          <Input
            id={id}
            type="number"
            inputMode="decimal"
            min={0}
            step={0.01}
            value={text}
            placeholder="不限"
            aria-invalid={error ? true : undefined}
            aria-describedby={describedBy}
            onChange={(e) => {
              setText(e.target.value);
              onChange(decimalOf(e.target.value));
            }}
            className="h-9 w-32 text-[14px] tnum"
          />
        </div>
      )}
    </Field>
  );
}

/** ModelSelect picks a model, or the one a value left empty gets (inherit says which). */
export function ModelSelect({ label, hint, value, onChange, inherit }: { label: ReactNode; hint?: ReactNode; value: string; onChange: (value: string) => void; inherit: string }) {
  const known = MODELS.some((m) => m.id === value);
  return (
    <Field label={label} hint={hint}>
      {(id, describedBy) => (
        <select id={id} value={value} aria-describedby={describedBy} onChange={(e) => onChange(e.target.value)} className={selectClass}>
          <option value="">{inherit}</option>
          {MODELS.map((m) => (
            <option key={m.id} value={m.id}>
              {m.label}
            </option>
          ))}
          {value && !known && <option value={value}>{value}</option>}
        </select>
      )}
    </Field>
  );
}

/** EffortSelect picks how hard a model thinks, or the one a value left empty gets. */
export function EffortSelect({ label, hint, value, onChange, inherit, efforts }: { label: ReactNode; hint?: ReactNode; value: string; onChange: (value: string) => void; inherit: string; efforts: string[] }) {
  return (
    <Field label={label} hint={hint}>
      {(id, describedBy) => (
        <select id={id} value={value} aria-describedby={describedBy} onChange={(e) => onChange(e.target.value)} className={selectClass}>
          <option value="">{inherit}</option>
          {efforts.map((e) => (
            <option key={e} value={e}>
              {EFFORT_WORDS[e] ?? e}（{e}）
            </option>
          ))}
          {value && !efforts.includes(value) && <option value={value}>{value}</option>}
        </select>
      )}
    </Field>
  );
}

export function SelectField({ label, hint, value, onChange, options }: { label: ReactNode; hint?: ReactNode; value: string; onChange: (value: string) => void; options: { value: string; label: string }[] }) {
  return (
    <Field label={label} hint={hint}>
      {(id, describedBy) => (
        <select id={id} value={value} aria-describedby={describedBy} onChange={(e) => onChange(e.target.value)} className={selectClass}>
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )}
    </Field>
  );
}
