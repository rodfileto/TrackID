export function Toggle({ label, checked, onChange, disabled }: { label: React.ReactNode; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <label className={`flex cursor-pointer items-center gap-2 text-base text-gray-800 dark:text-white/90 ${disabled ? "opacity-50" : ""}`}>
      <input type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} className="h-4 w-4 accent-brand-500" />
      {label}
    </label>
  );
}

export function Range({ label, value, min, max, step, unit, onChange }: {
  label: string; value: number; min: number; max: number; step: number; unit?: string; onChange: (v: number) => void;
}) {
  return (
    <label className="grid grid-cols-[9rem_1fr_5.5rem] items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
      <span>{label}</span>
      <input type="range" min={min} max={max} step={step} value={value} onChange={(e) => onChange(Number(e.target.value))} className="accent-brand-500" />
      <span className="flex items-center gap-1">
        <input
          type="number" min={min} max={max} step={step} value={value}
          onChange={(e) => e.target.value !== "" && onChange(Math.min(max, Math.max(min, Number(e.target.value))))}
          className="w-full rounded border border-gray-300 bg-transparent px-1 py-0.5 text-right text-sm dark:border-gray-700 dark:text-white/90"
        />
        {unit}
      </span>
    </label>
  );
}

export function Section({ title, children }: { title?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2 border-t border-gray-100 pt-3 first:border-t-0 first:pt-0 dark:border-gray-800">
      {title && <span className="text-base font-medium text-gray-800 dark:text-white/90">{title}</span>}
      {children}
    </div>
  );
}

/** A stability rating (H/M/L); with a label, the factor it is under. Dimmed when the factor does
 * not apply to the pair. */
export function StabilityChip({ value, title, label, dim }: { value?: string; title?: string; label?: string; dim?: boolean }) {
  const colour = value === "H" ? "bg-success-50 text-success-700 dark:bg-success-500/15 dark:text-success-400"
    : value === "M" ? "bg-warning-50 text-warning-700 dark:bg-warning-500/15 dark:text-warning-400"
    : value === "L" ? "bg-error-50 text-error-700 dark:bg-error-500/15 dark:text-error-400"
    : "bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400";
  return (
    <span title={title} className={`inline-flex h-6 min-w-6 items-center justify-center gap-1 rounded px-1.5 text-xs ${colour} ${dim ? "opacity-40" : ""}`}>
      {label && <span className="font-normal">{label}</span>}
      <span className="font-semibold">{value || "–"}</span>
    </span>
  );
}
