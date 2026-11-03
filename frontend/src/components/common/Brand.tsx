/** The TrackID mark, with the name unless iconOnly. inverse sets the name in white, for dark
 * panels whatever the theme. */
export default function Brand({ iconOnly = false, inverse = false, size = 32 }: { iconOnly?: boolean; inverse?: boolean; size?: number }) {
  return (
    <span className="flex items-center gap-2.5">
      <img src="/images/logo/logo-icon.svg" width={size} height={size} alt="" />
      {!iconOnly && (
        <span
          className={`font-semibold tracking-tight ${inverse ? "text-white" : "text-gray-900 dark:text-white"}`}
          style={{ fontSize: size * 0.68 }}
        >
          TrackID
        </span>
      )}
    </span>
  );
}
