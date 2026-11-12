// Serializable description of a table: server components build it with
// translated, formatted text and hand it to the DataTable client component.

export type Tone = "win" | "loss" | "muted" | "team-0" | "team-1";

export interface Cell {
  /** Display text, already localized and formatted. */
  text: string;
  /** Sort value: numbers sort numerically; defaults to text. */
  sort?: string | number;
  href?: string;
  tone?: Tone;
  /** Render as a pill (results, statuses). */
  badge?: boolean;
  /** Item slot colour mark: weapon | vitality | spirit | ability. */
  slot?: string;
  /** Strike through (sold items). */
  strike?: boolean;
  /** 0..1: a small bar before the text (win rates). */
  bar?: number;
  title?: string;
  /** Extra pill after the text, e.g. "left". */
  tag?: string;
  tagTitle?: string;
}

export interface Column {
  key: string;
  label: string;
  numeric?: boolean;
  /** Default true. */
  sortable?: boolean;
}

export interface Row {
  id: string;
  cells: Record<string, Cell>;
  /** Highlighted row (the selected player). */
  me?: boolean;
  /** Text the search box matches against. */
  search?: string;
  /** Values for the select filters, by filter key. */
  filters?: Record<string, string>;
}

export interface FilterOption {
  value: string;
  label: string;
}

export interface Filter {
  key: string;
  label: string;
  options: FilterOption[];
  /** "Any" option text. */
  anyLabel: string;
}

export interface TableSpec {
  /** Accessible name; also used by e2e tests to find the table. */
  label: string;
  /** Visible heading above the table (when several tables share a section). */
  caption?: { text: string; tone?: Tone; note?: string };
  columns: Column[];
  rows: Row[];
  initialSort?: { key: string; dir: "asc" | "desc" };
  /** Placeholder of the search box; no box when omitted. */
  searchPlaceholder?: string;
  filters?: Filter[];
  /** Show this many rows and a "show all" button. */
  collapseAfter?: number;
}
