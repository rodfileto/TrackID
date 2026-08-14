interface ProgressBarProps {
  percent: number; // 0-100
  label?: string;
  className?: string;
}

const ProgressBar: React.FC<ProgressBarProps> = ({ percent, label, className = "" }) => {
  const clamped = Math.min(100, Math.max(0, percent));

  return (
    <div className={className}>
      {label && (
        <div className="mb-1.5 flex items-center justify-between text-sm text-gray-600 dark:text-gray-400">
          <span>{label}</span>
          <span>{clamped.toFixed(0)}%</span>
        </div>
      )}
      <div className="h-2 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-white/10">
        <div
          className="h-full rounded-full bg-brand-500 transition-all duration-300 ease-linear"
          style={{ width: `${clamped}%` }}
        />
      </div>
    </div>
  );
};

export default ProgressBar;
