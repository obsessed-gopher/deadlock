export function Stat({ label, value, hint, meter }: { label: string; value: string; hint?: string; meter?: number }) {
  return (
    <div className="stat">
      <span className="stat-label">{label}</span>
      <span className="stat-value">{value}</span>
      {meter !== undefined && (
        <span className="meter" aria-hidden="true">
          <span style={{ width: `${Math.round(Math.max(0, Math.min(1, meter)) * 100)}%` }} />
        </span>
      )}
      {hint && <span className="stat-hint">{hint}</span>}
    </div>
  );
}
