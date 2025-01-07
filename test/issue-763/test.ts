import { assertEquals } from "jsr:@std/assert";

Deno.test("axios@1.6", async () => {
  const { default: axios } = await import("http://localhost:8080/axios@1.6");
  const res = await axios.get("http://localhost:8080/status.json");
  assertEquals(typeof res.data.version, "string");
});

Deno.test("axios@1.7", async () => {
  const { default: axios } = await import("http://localhost:8080/axios@1.7");
  const res = await axios.get("http://localhost:8080/status.json");
  assertEquals(typeof res.data.version, "string");
});
