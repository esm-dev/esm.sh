import { assertEquals } from "jsr:@std/assert";

Deno.test("issue #599", async () => {
  Reflect.set(globalThis, "window", globalThis);
  const { customElement } = await import(
    "http://localhost:8080/lit@2.7.2/decorators"
  );
  const { map } = await import(
    "http://localhost:8080/lit@2.7.2/directives/map"
  );
  assertEquals(typeof customElement, "function");
  assertEquals(typeof map, "function");
});
