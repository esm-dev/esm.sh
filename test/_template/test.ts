import { assert, assertEquals, assertStringIncludes } from "assert";

// change the import path to the module you want to test
import * as mod from "~/PKG[@SEMVER][/PATH]";

// change the test name and the test assertions
Deno.test("foo", () => {
  assert("foo" in mod);
  assertEquals(typeof mod.foo, "function");
  assertStringIncludes(mod.foo(), "bar");
});
