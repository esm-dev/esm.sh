import { assertEquals } from "jsr:@std/assert";

import axios from "http://localhost:8080/axios@1.3.4";

Deno.test("axios", async () => {
  const res = await axios.get("http://localhost:8080/status.json");
  assertEquals(typeof res.data.version, "number");
});
