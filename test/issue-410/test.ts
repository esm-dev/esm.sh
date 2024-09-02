import { assertEquals } from "jsr:@std/assert";

import Conf from "http://localhost:8080/conf@10.2.0";

Deno.test("issue #410", () => {
  const config = new Conf({ projectName: "test" });
  config.set("unicorn", "🦄");
  assertEquals(config.get("unicorn"), "🦄");
});
