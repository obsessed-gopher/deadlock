import assert from "node:assert/strict";
import { test } from "node:test";

import { ApiError, attempt, errorKind, kindOf } from "./errors.ts";

test("gateway responses map to error kinds", () => {
  assert.equal(kindOf(404, 5), "notFound");
  assert.equal(kindOf(400, 3), "badRequest");
  assert.equal(kindOf(503, 14), "unavailable");
  assert.equal(kindOf(0), "unavailable");
  assert.equal(kindOf(502), "unavailable");
  assert.equal(kindOf(500, 13), "internal");
  assert.equal(kindOf(500, 2), "internal");
  // The gRPC code wins over a generic HTTP status.
  assert.equal(kindOf(500, 14), "unavailable");
});

test("unknown errors are internal", () => {
  assert.equal(errorKind(new ApiError("notFound", 404, "not found")), "notFound");
  assert.equal(errorKind(new TypeError("boom")), "internal");
  assert.equal(errorKind("string"), "internal");
});

test("attempt turns failures into values", async () => {
  assert.deepEqual(await attempt(async () => 7), { ok: true, data: 7 });
  assert.deepEqual(
    await attempt(async () => {
      throw new ApiError("unavailable", 0, "fetch failed");
    }),
    { ok: false, kind: "unavailable" },
  );
});
