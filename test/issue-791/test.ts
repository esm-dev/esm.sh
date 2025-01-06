import { assertEquals } from "jsr:@std/assert";

Deno.test("issue #791", async () => {
  const res = await fetch("http://localhost:8080/@vueuse/shared@10.7.2");
  res.body?.cancel();
  assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/@vueuse/shared@10.7.2/index.d.cts");
});
