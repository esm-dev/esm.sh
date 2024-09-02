import { assertEquals } from "jsr:@std/assert";

import { createTheme } from "http://localhost:8080/@mui/material@5.15.15?external=react,react-dom&target=es2020";

Deno.test("issue #817", async () => {
  assertEquals(typeof createTheme, "function");
});
