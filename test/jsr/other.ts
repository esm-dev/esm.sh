import { assertExists } from "jsr:@std/assert";

Deno.test("jsr:@bids/schema", async () => {
  const { schema } = await import("http://localhost:8080/jsr/@bids/schema@0.11.3+2");
  assertExists(schema.objects);
});
