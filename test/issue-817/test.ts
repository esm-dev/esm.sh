import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { createTheme } from "http://localhost:8080/@mui/material@5.15.15?external=react,react-dom&target=es2020";

Deno.test("issue #817", async () => {
  assertEquals(typeof createTheme, "function");
});
