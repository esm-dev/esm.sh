import { assert, assertEquals } from "jsr:@std/assert";

// change the import path to the module you want to test
import { parse } from "http://localhost:8080/dotenv@17.2.0";

// related issue: https://github.com/esm-dev/esm.sh/issues/1161
Deno.test("testing name", () => {
  const config = parse("BASIC=basic"); // will return an object
  assertEquals(typeof config, "object");
  assert(config);
  assertEquals(config.BASIC, "basic");
});
