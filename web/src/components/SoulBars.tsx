import type { StatSnapshot } from "@/api/schema.gen.ts";
import type { I18n } from "@/i18n/core.ts";

/** Where the souls came from, as bars sorted by size (not a table: 5 rows). */
export function SoulBars({ i18n, s }: { i18n: I18n; s: StatSnapshot }) {
  const { t } = i18n;
  const sources = [
    { label: t("souls.laneCreeps"), v: s.goldLaneCreep },
    { label: t("souls.neutrals"), v: s.goldNeutralCreep },
    { label: t("souls.heroes"), v: s.goldPlayer },
    { label: t("souls.bosses"), v: s.goldBoss },
    { label: t("souls.treasure"), v: s.goldTreasure },
  ].sort((a, b) => b.v - a.v);
  const total = sources.reduce((sum, x) => sum + x.v, 0);
  const max = Math.max(1, ...sources.map((x) => x.v));
  return (
    <div className="card bars">
      {sources.map((x) => (
        <div key={x.label} className="line">
          <span>{x.label}</span>
          <span className="track" aria-hidden="true">
            <span style={{ width: `${(x.v / max) * 100}%` }} />
          </span>
          <span className="value">
            {i18n.num(x.v)} <span className="faint">{i18n.pct(x.v, total)}</span>
          </span>
        </div>
      ))}
      <div className="line tone-loss">
        <span>{t("souls.deathLoss")}</span>
        <span />
        <span className="value">−{i18n.num(s.goldDeathLoss)}</span>
      </div>
    </div>
  );
}
