import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

// `temporal-polyfill` has a non-empty `sideEffects` field, so its sub-modules are not bundled.
// `getAny` writes to a module-level WeakMap that `withCalendar` reads back. Two copies of that
// module mean two maps, and the read then throws `Invalid calling context`.
import * as PlainDateFns from "http://localhost:8080/temporal-polyfill@1.0.4/fns/PlainDate?target=es2022";
import { getAny } from "http://localhost:8080/temporal-polyfill@1.0.4/fns/Calendar?target=es2022";

Deno.test("splitting sub-modules are shared by export modules", async () => {
  const res = await fetch("http://localhost:8080/svelte@5.16.0?target=es2022");
  const text = await res.text();
  assertStringIncludes(text, "src/internal/client/render.mjs");
});

Deno.test("splitting sub-modules are shared by packages with a `sideEffects` field", () => {
  const date = PlainDateFns.create(2026, 9, 6);
  const withCalendar = PlainDateFns.withCalendar(date, getAny("iso8601"));
  assertEquals(PlainDateFns.toString(withCalendar), "2026-09-06");
});
