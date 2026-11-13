"use client";

// The one table component: sorting by any column, search, select filters and
// collapsing of long tables. Every table in the app goes through it (except
// the server-paginated match history, which sorts and filters on the server).
import Link from "next/link";
import { useId, useMemo, useState } from "react";

import { useI18n } from "@/i18n/client.tsx";
import { type SortState, filterRows, nextSort, sortRows, visibleRows } from "@/lib/table.ts";

import type { Cell, Row, TableSpec } from "@/lib/tableSpec.ts";

import { Avatar } from "./Avatar.tsx";
import { Icon } from "./Icon.tsx";
import { Select } from "./Select.tsx";

export function DataTable({ spec }: { spec: TableSpec }) {
  const { t, locale } = useI18n();
  const id = useId();
  const [sort, setSort] = useState<SortState | null>(spec.initialSort ?? null);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [expanded, setExpanded] = useState(false);

  const filtered = useMemo(() => {
    const rows = filterRows(spec.rows, {
      query,
      searchText: (r: Row) => r.search ?? "",
      filters: (spec.filters ?? []).map((f) => ({ key: f.key, value: (r: Row) => r.filters?.[f.key] ?? "" })),
      selected,
      locale,
    });
    if (!sort) return rows;
    return sortRows(rows, (r) => sortValue(r.cells[sort.key]), sort.dir, locale);
  }, [spec.rows, spec.filters, query, selected, sort, locale]);

  const shown = visibleRows(filtered, spec.collapseAfter, expanded);
  const hasTools = spec.searchPlaceholder !== undefined || (spec.filters?.length ?? 0) > 0;

  return (
    <div className="dt">
      {spec.caption && (
        <h3 className="dt-caption">
          <span className={spec.caption.tone ? `tone-${spec.caption.tone}` : undefined}>{spec.caption.text}</span>
          {spec.caption.note && <span className="faint"> · {spec.caption.note}</span>}
        </h3>
      )}
      {hasTools && (
        <div className="toolbar">
          {spec.searchPlaceholder !== undefined && (
            <label className="field search">
              <SearchIcon />
              <input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={spec.searchPlaceholder}
                aria-label={`${spec.label}: ${spec.searchPlaceholder}`}
              />
            </label>
          )}
          {spec.filters?.map((f) => (
            <Select
              key={f.key}
              label={f.label}
              ariaLabel={`${spec.label}: ${f.label}`}
              value={selected[f.key] ?? ""}
              onChange={(v) => setSelected((s) => ({ ...s, [f.key]: v }))}
              options={[{ value: "", label: f.anyLabel }, ...f.options]}
            />
          ))}
        </div>
      )}
      <div className="table-wrap">
        <table aria-label={spec.label} id={id}>
          <thead>
            <tr>
              {spec.columns.map((c) => {
                const active = sort?.key === c.key;
                const ariaSort = active ? (sort.dir === "asc" ? "ascending" : "descending") : "none";
                return (
                  <th key={c.key} className={c.numeric ? "num" : undefined} aria-sort={c.sortable === false ? undefined : ariaSort}>
                    {c.sortable === false ? (
                      c.label
                    ) : (
                      <button
                        type="button"
                        className={active ? "sort active" : "sort"}
                        onClick={() => setSort(nextSort(sort, c.key, !!c.numeric))}
                        title={t("table.sortBy", { col: c.label })}
                      >
                        {c.label}
                        <span className="arrow" aria-hidden="true">
                          {active ? (sort.dir === "asc" ? "↑" : "↓") : "↕"}
                        </span>
                      </button>
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {shown.map((r) => (
              <tr key={r.id} className={r.me ? "me" : undefined}>
                {spec.columns.map((c) => (
                  <td key={c.key} className={c.numeric ? "num" : undefined}>
                    <CellView cell={r.cells[c.key]} />
                  </td>
                ))}
              </tr>
            ))}
            {shown.length === 0 && (
              <tr>
                <td colSpan={spec.columns.length} className="empty">
                  {t("table.noRows")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {spec.collapseAfter !== undefined && filtered.length > spec.collapseAfter && (
          <button type="button" className="more" aria-controls={id} onClick={() => setExpanded((x) => !x)}>
            {expanded ? t("table.collapse") : t("table.showAll", { n: filtered.length })}
          </button>
        )}
      </div>
    </div>
  );
}

function sortValue(cell: Cell | undefined): string | number {
  return cell?.sort ?? cell?.text ?? "";
}

function CellView({ cell }: { cell: Cell | undefined }) {
  if (!cell) return null;
  const cls = [cell.tone ? `tone-${cell.tone}` : "", cell.strike ? "strike" : ""].filter(Boolean).join(" ") || undefined;
  let body = cell.href ? (
    <Link href={cell.href} className={cls}>
      {cell.text}
    </Link>
  ) : (
    <span className={cls} title={cell.title}>
      {cell.text}
    </span>
  );
  if (cell.badge) {
    body = (
      <span className={`badge ${cell.tone ? `tone-${cell.tone}` : ""}`} title={cell.title}>
        <span className="dot" aria-hidden="true" />
        {cell.text}
      </span>
    );
  }
  return (
    <span className="cell">
      {cell.slot && <span className={`slot-mark slot-${cell.slot}`} aria-hidden="true" />}
      {cell.avatar && <Avatar src={cell.avatar.src} name={cell.avatar.name} size={24} />}
      {cell.icon && <Icon src={cell.icon} tile={cell.iconTile} />}
      {cell.bar !== undefined && (
        <span className={cell.tone ? `bar tone-${cell.tone}` : "bar"} aria-hidden="true">
          <span style={{ width: `${Math.round(Math.max(0, Math.min(1, cell.bar)) * 100)}%` }} />
        </span>
      )}
      {body}
      {cell.tag && (
        <span className="tag" title={cell.tagTitle}>
          {cell.tag}
        </span>
      )}
    </span>
  );
}

export function SearchIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" />
    </svg>
  );
}
