import assert from "node:assert/strict";
import { test } from "node:test";

import { parseAccounts, toAccountId } from "./tracked.ts";

test("SYNC_ACCOUNTS: account ids and SteamID64, deduplicated, garbage dropped", () => {
  assert.deepEqual(parseAccounts(" 1517505834, 76561199477771562 ,x,,0, 42"), ["1517505834", "42"]);
  assert.deepEqual(parseAccounts(undefined), []);
  assert.equal(toAccountId("4294967296"), null);
});
