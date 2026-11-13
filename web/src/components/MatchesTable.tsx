import Link from "next/link";

import { PAGE_SIZE } from "@/api/client.ts";
import type { ListPlayerMatchesResponse, MetaStatus } from "@/api/schema.gen.ts";
import type { I18n } from "@/i18n/core.ts";
import { kda } from "@/lib/format.ts";
import type { Lookup } from "@/lib/gamedata.ts";
import { QUEUES, queueOf } from "@/lib/queue.ts";

import { Icon } from "./Icon.tsx";
import { Select } from "./Select.tsx";
import {
  type MatchListState,
  type MatchSortKey,
  type MetaFilter,
  type ResultFilter,
  DEFAULT_STATE,
  filterHref,
  nextPageHref,
  sortHref,
} from "@/lib/matchQuery.ts";

const metaKey: Record<MetaStatus, "meta.ok" | "meta.pending" | "meta.expired" | "meta.failed" | null> = {
  META_STATUS_UNSPECIFIED: null,
  META_STATUS_OK: "meta.ok",
  META_STATUS_PENDING: "meta.pending",
  META_STATUS_EXPIRED: "meta.expired",
  META_STATUS_FAILED: "meta.failed",
};

/**
 * Match history: sorted, filtered and paged by the server, so every control is
 * a link or a GET form; the URL holds the whole state (lib/matchQuery.ts).
 */
export function MatchesTable({
  i18n,
  look,
  base,
  accountId,
  state,
  page,
}: {
  i18n: I18n;
  look: Lookup;
  base: string;
  accountId: number;
  state: MatchListState;
  page: ListPlayerMatchesResponse;
}) {
  const { t } = i18n;
  const cols: { key: MatchSortKey | null; label: string; numeric?: boolean }[] = [
    { key: "date", label: t("col.date") },
    { key: null, label: t("col.hero") },
    { key: null, label: t("col.result") },
    { key: "kills", label: t("col.kdaShort"), numeric: true },
    { key: "souls", label: t("col.souls"), numeric: true },
    { key: "lastHits", label: t("col.lastHits"), numeric: true },
    { key: "level", label: t("col.level"), numeric: true },
    { key: "duration", label: t("col.duration"), numeric: true },
    { key: null, label: t("col.mode") },
    { key: null, label: t("col.status") },
  ];
  const results: [ResultFilter, string][] = [
    ["", t("filter.all")],
    ["win", t("result.wins")],
    ["loss", t("result.losses")],
  ];
  const metas: [MetaFilter, string][] = [
    ["ok", t("meta.ok")],
    ["pending", t("meta.pending")],
    ["expired", t("meta.expired")],
    ["failed", t("meta.failed")],
  ];
  const filtered = state.hero || state.result || state.mode || state.meta;
  const from = page.matches.length ? state.from + 1 : 0;

  return (
    <section aria-labelledby="matches-title">
      <div className="section-head">
        <h2 id="matches-title">
          {t("matches.title")}
          <span className="count">{i18n.num(page.total)}</span>
        </h2>
        <nav className="segmented" aria-label={t("filter.result")}>
          {results.map(([v, label]) => (
            <Link key={v || "all"} href={filterHref(base, state, { result: v })} aria-current={state.result === v}>
              {label}
            </Link>
          ))}
        </nav>
      </div>

      {/* GET form: filters become query parameters; the page reloads with them. */}
      <form method="get" action={base} className="toolbar" aria-label={t("matches.title")}>
        {state.sort !== DEFAULT_STATE.sort && <input type="hidden" name="sort" value={state.sort} />}
        {state.dir !== DEFAULT_STATE.dir && <input type="hidden" name="dir" value={state.dir} />}
        {state.result && <input type="hidden" name="result" value={state.result} />}
        <Select
          name="hero"
          label={t("filter.hero")}
          defaultValue={state.hero ? String(state.hero) : ""}
          options={[
            { value: "", label: t("filter.all") },
            ...look.playableHeroes
              .map((h) => ({ value: String(h.id), label: look.heroName(h.id), icon: look.heroIcon(h.id) }))
              .sort((a, b) => a.label.localeCompare(b.label, i18n.locale)),
          ]}
        />
        <Select
          name="mode"
          label={t("filter.mode")}
          defaultValue={state.mode}
          options={[{ value: "", label: t("filter.all") }, ...QUEUES.map((q) => ({ value: q, label: t(`queue.${q}`) }))]}
        />
        <Select
          name="meta"
          label={t("filter.meta")}
          defaultValue={state.meta}
          options={[{ value: "", label: t("filter.any") }, ...metas.map(([v, label]) => ({ value: v, label }))]}
        />
        <button type="submit" className="btn small">
          {t("filter.apply")}
        </button>
        {filtered ? (
          <Link href={filterHref(base, state, { hero: 0, result: "", mode: "", meta: "" })} className="btn small">
            {t("filter.reset")}
          </Link>
        ) : null}
      </form>

      <div className="table-wrap">
        <table aria-label={t("matches.title")}>
          <thead>
            <tr>
              {cols.map((c) => {
                const active = c.key !== null && state.sort === c.key;
                return (
                  <th
                    key={c.label}
                    className={c.numeric ? "num" : undefined}
                    aria-sort={c.key ? (active ? (state.dir === "asc" ? "ascending" : "descending") : "none") : undefined}
                  >
                    {c.key ? (
                      <Link
                        href={sortHref(base, state, c.key)}
                        className={active ? "sort active" : "sort"}
                        title={t("table.sortBy", { col: c.label })}
                      >
                        {c.label}
                        <span className="arrow" aria-hidden="true">
                          {active ? (state.dir === "asc" ? "↑" : "↓") : "↕"}
                        </span>
                      </Link>
                    ) : (
                      c.label
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {page.matches.map((m) => {
              const meta = metaKey[m.metaStatus];
              return (
                <tr key={m.matchId}>
                  <td className="faint mono small">{i18n.date(m.startTime)}</td>
                  <td>
                    <span className="cell">
                      {look.heroIcon(m.heroId) && <Icon src={look.heroIcon(m.heroId)!} />}
                      {look.heroName(m.heroId)}
                    </span>
                  </td>
                  <td>
                    <span className={`badge ${m.won ? "tone-win" : "tone-loss"}`}>
                      <span className="dot" aria-hidden="true" />
                      {m.won ? t("result.win") : t("result.loss")}
                    </span>
                  </td>
                  <td className="num">{kda(m.kills, m.deaths, m.assists)}</td>
                  <td className="num">{i18n.num(m.netWorth)}</td>
                  <td className="num">{m.lastHits}</td>
                  <td className="num">{m.heroLevel}</td>
                  <td className="num">{i18n.duration(m.durationS)}</td>
                  <td
                    className="faint small"
                    title={`${i18n.gameMode(m.gameMode)} · ${i18n.matchMode(m.matchMode)}`}
                  >
                    {t(`queue.${queueOf(m.gameMode, m.matchMode)}`)}
                  </td>
                  <td>
                    {m.metaStatus === "META_STATUS_OK" ? (
                      <Link href={`/matches/${m.matchId}/players/${accountId}`}>{t("matches.breakdown")}</Link>
                    ) : (
                      <span className="faint small">{meta ? t(meta) : ""}</span>
                    )}
                  </td>
                </tr>
              );
            })}
            {page.matches.length === 0 && (
              <tr>
                <td colSpan={cols.length} className="empty">
                  {t("matches.empty")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="pager">
        <span>{t("matches.range", { from, to: state.from + page.matches.length, total: page.total })}</span>
        <div className="row">
          {state.page && (
            <Link className="btn small" href={filterHref(base, state, {})}>
              {t("matches.first")}
            </Link>
          )}
          {page.nextPageToken && (
            <Link className="btn small" href={nextPageHref(base, state, page.nextPageToken, PAGE_SIZE)}>
              {t("matches.next")}
            </Link>
          )}
        </div>
      </div>
    </section>
  );
}
