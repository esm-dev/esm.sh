import { assertEquals } from "jsr:@std/assert";

import Asciidoctor from "http://localhost:8080/asciidoctor@3.0.0-alpha.4";

Deno.test("issue #557", () => {
  assertEquals(typeof Asciidoctor, "function");
});
