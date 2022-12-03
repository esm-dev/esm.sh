import { assertEquals } from "https://deno.land/std@0.162.0/testing/asserts.ts";

import {createTheme} from "http://localhost:8080/baseui@12.2.0";

Deno.test("baseui", () => {
  assertEquals(typeof createTheme, "function");
});

