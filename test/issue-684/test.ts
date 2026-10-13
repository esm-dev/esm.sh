import { assertEquals, assertInstanceOf, assertRejects } from "jsr:@std/assert";

for (const query of ["?deps=pg-pool@3.10.1", "?bundle"]) {
  Deno.test(`pg uses its JavaScript client without native bindings (${query})`, async () => {
    const { default: pg, Client, Pool } = await import(
      "http://localhost:8080/pg@8.23.1" + query
    );
    assertEquals(Client, pg.Client);
    assertEquals(Pool, pg.Pool);
    assertInstanceOf(new Client(), Client);
    assertInstanceOf(new Pool(), Pool);
    assertEquals(pg.escapeIdentifier('a"b'), '"a""b"');
    assertEquals(pg.native, null);
  });
}

Deno.test("pg reports missing bindings only when native mode is requested", async () => {
  const url = "http://localhost:8080/pg@8.23.1/lib/native/index.js";
  const error = await assertRejects(
    () => import(url),
    Error,
    "Cannot find module 'pg-native'",
  );
  assertEquals((error as Error & { code: string }).code, "MODULE_NOT_FOUND");
});
