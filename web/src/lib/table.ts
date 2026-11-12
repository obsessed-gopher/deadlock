// Client-side table logic: sorting, searching, filtering, collapsing.
// Pure functions shared by every table component (components/DataTable.tsx).

export type SortDir = "asc" | "desc";
export type SortValue = string | number;

export interface SortState {
  key: string;
  dir: SortDir;
}

/**
 * Stable sort by a value getter. Strings compare with the locale's collation,
 * numbers numerically; equal values keep their original order.
 */
export function sortRows<T>(rows: readonly T[], value: (r: T) => SortValue, dir: SortDir, locale = "en"): T[] {
  const collator = new Intl.Collator(locale, { numeric: true, sensitivity: "base" });
  const sign = dir === "asc" ? 1 : -1;
  return rows
    .map((r, i) => ({ r, i, v: value(r) }))
    .sort((a, b) => {
      const c =
        typeof a.v === "number" && typeof b.v === "number" ? a.v - b.v : collator.compare(String(a.v), String(b.v));
      return c * sign || a.i - b.i;
    })
    .map((x) => x.r);
}

/**
 * Header click: the same column flips direction; another column starts with
 * its natural direction (numbers: largest first, text: A to Z).
 */
export function nextSort(current: SortState | null, key: string, numeric: boolean): SortState {
  if (current?.key === key) return { key, dir: current.dir === "asc" ? "desc" : "asc" };
  return { key, dir: numeric ? "desc" : "asc" };
}

/** Case- and accent-insensitive "contains" over the row's search text. */
export function matchesSearch(text: string, query: string, locale = "en"): boolean {
  const q = query.trim();
  if (!q) return true;
  const norm = (s: string) =>
    s
      .toLocaleLowerCase(locale)
      .normalize("NFD")
      .replace(/\p{Diacritic}/gu, "");
  return norm(text).includes(norm(q));
}

export interface FilterDef<T> {
  key: string;
  /** Row's value for this filter; the row passes when it equals the selection. */
  value: (r: T) => string;
}

/** Rows passing the search and every selected filter ("" = no filter). */
export function filterRows<T>(
  rows: readonly T[],
  opts: {
    query?: string;
    searchText?: (r: T) => string;
    filters?: readonly FilterDef<T>[];
    selected?: Readonly<Record<string, string>>;
    locale?: string;
  },
): T[] {
  const { query = "", searchText, filters = [], selected = {}, locale } = opts;
  return rows.filter((r) => {
    if (searchText && !matchesSearch(searchText(r), query, locale)) return false;
    return filters.every((f) => {
      const want = selected[f.key] ?? "";
      return want === "" || f.value(r) === want;
    });
  });
}

/** Rows to render: all when expanded or short, else the first `limit`. */
export function visibleRows<T>(rows: readonly T[], limit: number | undefined, expanded: boolean): T[] {
  return !limit || expanded || rows.length <= limit ? [...rows] : rows.slice(0, limit);
}
