import { assertEquals } from "jsr:@std/assert";

import { Hono } from "http://localhost:8080/jsr/@hono/hono@4";

Deno.test("hono", async () => {
  const hono = new Hono();
  hono.get("/", (ctx) => ctx.text("Hello, Hono!"));
  assertEquals(await (await hono.fetch(new Request("http://localhost/"))).text(), "Hello, Hono!");
});
