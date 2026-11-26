import { assertEquals, assertStrictEquals } from "jsr:@std/assert";

const base = "http://localhost:8080/langsmith@0.11.7";

for (const target of ["deno", "denonext"]) {
  Deno.test(`issue #1508 - distinct client exports (${target})`, async () => {
    const root = await import(`${base}?target=${target}`);
    const sibling = await import(`${base}/client.mjs?target=${target}`);
    assertEquals(typeof root.Langsmith, "function");
    assertStrictEquals(root.Langsmith, sibling.Langsmith);

    const client = await import(`${base}/client?target=${target}`);
    const canonical = await import(`${base}/${target}/client.mjs`);
    assertEquals(typeof client.Client, "function");
    assertStrictEquals(client.Client, canonical.Client);
  });
}
