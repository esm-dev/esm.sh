import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";
import { Document } from "https://deno.land/x/deno_dom@v0.1.38/deno-dom-wasm.ts";

// add virtual document object to globalThis which is required by html-dom-parser
Reflect.set(globalThis, "document", new Document());

const { default: HTMLDOMParser } = await import(
  "http://localhost:8080/html-dom-parser@3.1.7"
);

Deno.test("issue #577", () => {
  const dom = HTMLDOMParser("<p>Hello, World!</p>");
  assertEquals(dom[0].type, "tag");
});
