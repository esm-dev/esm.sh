import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { createTheme } from "http://localhost:8080/baseui@12.2.0";

Deno.test("baseui", () => {
  assertEquals(typeof createTheme, "function");
});
