import { assertEquals, assertStrictEquals } from "jsr:@std/assert";

const origin = "http://localhost:8080";

Deno.test("issue #1344 - nested Zod exports and SDK client", async () => {
  const z = await import(`${origin}/zod@4.6.7/v4`);
  const { Client } = await import(
    `${origin}/@modelcontextprotocol/sdk@1.29.0/dist/esm/client`
  );
  const schema = z.object({ name: z.string(), value: z.custom() });
  assertEquals(schema.parse({ name: "test", value: 42 }), {
    name: "test",
    value: 42,
  });
  assertStrictEquals(z.string, z.default.string);
  assertStrictEquals(z.ZodString, z.default.ZodString);
  assertEquals(
    new Client({ name: "test", version: "1.0.0" }) instanceof Client,
    true,
  );
});

Deno.test("issue #1130 - Zenfs backend exports", async () => {
  const { StoreFS, Transaction, configure, InMemory, fs } = await import(
    `${origin}/@zenfs/core@2.2.0?target=es2022`
  );
  assertEquals(typeof StoreFS, "function");
  assertEquals(typeof Transaction, "function");
  const { IndexedDB } = await import(
    `${origin}/@zenfs/dom@1.1.7/IndexedDB?target=es2022&deps=@zenfs/core@2.2.0`
  );
  assertEquals(typeof IndexedDB, "object");
  await configure({ mounts: { "/": InMemory } });
  fs.writeFileSync("/export-star.txt", "ok");
  assertEquals(fs.readFileSync("/export-star.txt", "utf8"), "ok");
});

Deno.test("issue #1196 - MUI virtualizer exports", async () => {
  const { EMPTY_RENDER_CONTEXT } = await import(
    `${origin}/@mui/x-virtualizer@0.1.2?target=es2022`
  );
  assertEquals(EMPTY_RENDER_CONTEXT, {
    firstRowIndex: 0,
    lastRowIndex: 0,
    firstColumnIndex: 0,
    lastColumnIndex: 0,
  });
});

Deno.test("issue #696 - Discordeno cyclic export stars", async () => {
  const bot = await import(
    `${origin}/@discordeno/bot@19.0.0-next.3445211?target=denonext`
  );
  assertEquals(typeof bot.createBot, "function");
  assertEquals(typeof bot.iconHashToBigInt, "function");
  assertEquals(Object.keys(bot).length, 302);
});
