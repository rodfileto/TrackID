/** A user's initials in a circle, in place of a photo. */
export default function InitialsAvatar({ firstName, lastName, className = "h-11 w-11 text-sm" }: { firstName?: string; lastName?: string; className?: string }) {
  const initials = `${firstName?.trim()[0] ?? ""}${lastName?.trim()[0] ?? ""}`.toUpperCase() || "?";
  return (
    <span className={`flex shrink-0 items-center justify-center rounded-full bg-brand-500 font-semibold text-white ${className}`} aria-hidden="true">
      {initials}
    </span>
  );
}
