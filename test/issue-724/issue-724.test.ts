import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";
import addClass from "http://localhost:8080/v132/dom-helpers@3.4.0/es2022/class/addClass.js";

Deno.test("issue #724", () => {
  const el = { className: "foo" };
  addClass(el, "bar");
  assertEquals(
    el.className,
    "foo bar",
  );
});
