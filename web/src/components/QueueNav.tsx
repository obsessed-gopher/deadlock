// Queue switch (all / normal / ranked / Street Brawl / other) with match
// counts. Links, so the choice lives in the URL; empty queues are hidden
// unless selected.
import Link from "next/link";

import type { I18n } from "@/i18n/core.ts";
import { QUEUES, type QueueKey } from "@/lib/queue.ts";

export function QueueNav({
  i18n,
  counts,
  current,
  hrefFor,
}: {
  i18n: I18n;
  counts: Record<QueueKey, number>;
  current: QueueKey | "";
  hrefFor: (q: QueueKey | "") => string;
}) {
  const { t } = i18n;
  const total = QUEUES.reduce((n, q) => n + counts[q], 0);
  const shown = QUEUES.filter((q) => counts[q] > 0 || q === current);
  // One queue only: nothing to switch.
  if (shown.length <= 1 && !current) return null;
  return (
    <nav className="segmented queue-nav" aria-label={t("filter.mode")}>
      <Link href={hrefFor("")} aria-current={current === "" ? "true" : undefined} scroll={false}>
        {t("filter.all")} <span className="faint">{i18n.num(total)}</span>
      </Link>
      {shown.map((q) => (
        <Link key={q} href={hrefFor(q)} aria-current={current === q ? "true" : undefined} scroll={false}>
          {t(`queue.${q}`)} <span className="faint">{i18n.num(counts[q])}</span>
        </Link>
      ))}
    </nav>
  );
}
