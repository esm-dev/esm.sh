// related issue: https://github.com/esm-dev/esm.sh/issues/[ISSUE_NUMBER]

// change the import path to the module you want to test
import * as mod from "~/PKG[@SEMVER][/PATH]";
import { assert, assertEquals, assertStringIncludes } from "assert";

// change the test name and the test assertions
Deno.test("TEST NAME", async () => {
  assert("foo" in mod);
  assertEquals(typeof mod.foo, "function");
  assertStringIncludes(mod.foo(), "bar");
});
