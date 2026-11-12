import assert from "node:assert/strict";
import { test } from "node:test";

import { filterRows, matchesSearch, nextSort, sortRows, visibleRows } from "./table.ts";

const rows = [
  { name: "Haze", m: 114, slot: "spirit" },
  { name: "abrams", m: 12, slot: "weapon" },
  { name: "Wraith", m: 114, slot: "weapon" },
  { name: "Ivy", m: 9, slot: "spirit" },
];

test("numbers sort numerically and stably", () => {
  assert.deepEqual(
    sortRows(rows, (r) => r.m, "desc").map((r) => r.name),
    ["Haze", "Wraith", "abrams", "Ivy"],
  );
  assert.deepEqual(
    sortRows(rows, (r) => r.m, "asc").map((r) => r.name),
    ["Ivy", "abrams", "Haze", "Wraith"],
  );
});

test("text sorts by collation, ignoring case, numbers inside naturally", () => {
  assert.deepEqual(
    sortRows(rows, (r) => r.name, "asc").map((r) => r.name),
    ["abrams", "Haze", "Ivy", "Wraith"],
  );
  assert.deepEqual(
    sortRows(["item 10", "item 9"], (r) => r, "asc"),
    ["item 9", "item 10"],
  );
  assert.deepEqual(
    sortRows(["Ямато", "Апполон", "Ёж"], (r) => r, "asc", "ru"),
    ["Апполон", "Ёж", "Ямато"],
  );
});

test("sortRows does not mutate its input", () => {
  const copy = [...rows];
  sortRows(rows, (r) => r.m, "asc");
  assert.deepEqual(rows, copy);
});

test("header clicks: flip on the same column, natural direction on a new one", () => {
  assert.deepEqual(nextSort(null, "m", true), { key: "m", dir: "desc" });
  assert.deepEqual(nextSort(null, "name", false), { key: "name", dir: "asc" });
  assert.deepEqual(nextSort({ key: "m", dir: "desc" }, "m", true), { key: "m", dir: "asc" });
  assert.deepEqual(nextSort({ key: "m", dir: "asc" }, "m", true), { key: "m", dir: "desc" });
  assert.deepEqual(nextSort({ key: "m", dir: "asc" }, "name", false), { key: "name", dir: "asc" });
});

test("search ignores case, diacritics and surrounding spaces", () => {
  assert.ok(matchesSearch("Haze", " hA "));
  assert.ok(matchesSearch("Pocket", ""));
  assert.ok(matchesSearch("Ёж", "еж", "ru"));
  assert.ok(matchesSearch("Café", "cafe"));
  assert.ok(!matchesSearch("Haze", "wraith"));
});

test("filters combine with search; empty selection means no filter", () => {
  const f = [{ key: "slot", value: (r: (typeof rows)[number]) => r.slot }];
  assert.deepEqual(
    filterRows(rows, { filters: f, selected: { slot: "weapon" } }).map((r) => r.name),
    ["abrams", "Wraith"],
  );
  assert.deepEqual(
    filterRows(rows, { filters: f, selected: { slot: "" }, query: "a", searchText: (r) => r.name }).map((r) => r.name),
    ["Haze", "abrams", "Wraith"],
  );
  assert.deepEqual(filterRows(rows, { query: "zzz", searchText: (r) => r.name }), []);
});

test("collapsing shows the first rows until expanded", () => {
  assert.equal(visibleRows(rows, 2, false).length, 2);
  assert.equal(visibleRows(rows, 2, true).length, 4);
  assert.equal(visibleRows(rows, 10, false).length, 4);
  assert.equal(visibleRows(rows, undefined, false).length, 4);
});
