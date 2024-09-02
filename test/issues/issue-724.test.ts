import { assertEquals } from "jsr:@std/assert";
import addClass from "http://localhost:8080/dom-helpers@3.4.0/class/addClass?target=es2022";

Deno.test("issue #724", () => {
  const el: any = { className: "foo" };
  addClass(el, "bar");
  assertEquals(
    el.className,
    "foo bar",
  );
});
