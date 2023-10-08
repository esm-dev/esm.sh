import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";
import addClass from "http://localhost:8080/dom-helpers@3.4.0/class/addClass?target=es2022";

Deno.test("issue #724", () => {
  const el: any = { className: "foo" };
  addClass(el, "bar");
  assertEquals(
    el.className,
    "foo bar",
  );
});
