const sizes = {
  sm: "h-4 w-4",
  md: "h-5 w-5",
  lg: "h-8 w-8",
};

/** A turning ring for work under way. In the current text colour unless className says otherwise; with a label it
 * is announced to screen readers, without one it is decoration beside text that says what is happening. */
export default function Spinner({ size = "sm", label, className = "" }: { size?: keyof typeof sizes; label?: string; className?: string }) {
  return (
    <span role={label ? "status" : undefined} className={`inline-flex shrink-0 ${className}`}>
      <svg className={`${sizes[size]} animate-spin motion-reduce:animate-[spin_1.5s_linear_infinite]`} viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" className="opacity-25" />
        <path d="M22 12a10 10 0 0 0-10-10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
      </svg>
      {label && <span className="sr-only">{label}</span>}
    </span>
  );
}
