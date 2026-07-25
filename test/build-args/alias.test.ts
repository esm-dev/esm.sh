import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("types with ?alias", async () => {
  const url = "http://localhost:8080/@emotion/styled@11.11.5/types/base.d.ts";
  const plain = await fetch(url).then((res) => res.text());
  const queryRes = await fetch(
    url + "?alias=react:preact/compat&deps=preact@10.6.6",
  );
  assertEquals(queryRes.status, 200);
  const queryTypes = await queryRes.text();
  assertStringIncludes(queryTypes, "preact@10.6.6/compat/src/index.d.ts");
  assert(queryTypes !== plain);

  const res = await fetch(
    `http://localhost:8080/@emotion/styled@11.11.5/X-YXJlYWN0OnByZWFjdC9jb21wYXQKZHByZWFjdEAxMC42LjY/types/base.d.ts`,
  );
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("Content-type"),
    "application/typescript; charset=utf-8",
  );
  const ts = await res.text();
  assertStringIncludes(ts, "preact@10.6.6/compat/src/index.d.ts");
});
