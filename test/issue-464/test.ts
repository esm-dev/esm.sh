import { assertEquals } from "jsr:@std/assert";

class HTMLElement {}
Reflect.set(globalThis, "window", globalThis);
Reflect.set(globalThis, "HTMLElement", HTMLElement);
Reflect.set(globalThis, "document", { createTreeWalker: () => {} });

const { LitElement } = await import("http://localhost:8080/lit-element@3.2.1");

Deno.test("issue #464", () => {
  assertEquals(typeof LitElement, "function");
});
