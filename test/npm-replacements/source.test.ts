import { assertEquals, assertNotStrictEquals, assertRejects, assertStrictEquals } from "jsr:@std/assert";
import concatMap from "../../internal/npm_replacements/src/concat-map.mjs";
import arrayMap from "../../internal/npm_replacements/src/array-map.mjs";
import slice from "../../internal/npm_replacements/src/arraybuffer.prototype.slice.mjs";
import any from "../../internal/npm_replacements/src/promise.any.mjs";
import allSettled from "../../internal/npm_replacements/src/promise.allsettled.mjs";

Deno.test("concat-map replacement passes callback arguments and flattens one level", () => {
  const input = [1, 2, 3];
  assertEquals(
    concatMap(input, (value: number, index: number, array: number[]) => {
      assertStrictEquals(array, input);
      return index === 1 ? value : [value, [index]];
    }),
    [1, [0], 2, 3, [2]],
  );
  assertEquals(input, [1, 2, 3]);
});

Deno.test("array-map replacement passes callback arguments", () => {
  const input = [1, 2, 3];
  assertEquals(
    arrayMap(input, (value: number, index: number, array: number[]) => {
      assertStrictEquals(array, input);
      return value + index;
    }),
    [1, 3, 5],
  );
  assertEquals(input, [1, 2, 3]);
});

Deno.test("arraybuffer.prototype.slice replacement preserves slice bounds and copies", () => {
  const input = new Uint8Array([1, 2, 3, 4]).buffer;
  assertEquals(new Uint8Array(slice(input, 1, 3)), new Uint8Array([2, 3]));
  assertEquals(new Uint8Array(slice(input, -2)), new Uint8Array([3, 4]));
  const copy = slice(input);
  assertNotStrictEquals(copy, input);
  new Uint8Array(copy)[0] = 9;
  assertEquals(new Uint8Array(input), new Uint8Array([1, 2, 3, 4]));
});

Deno.test("promise.any replacement supports standalone calls and rejection", async () => {
  assertEquals(await any(new Set([Promise.reject("first"), Promise.resolve(2)])), 2);
  const error = await assertRejects(() => any([Promise.reject("first"), Promise.reject("second")]), AggregateError);
  assertEquals(error.errors, ["first", "second"]);
  await assertRejects(() => any([]), AggregateError);
});

Deno.test("promise.allsettled replacement supports standalone calls and mixed outcomes", async () => {
  assertEquals(await allSettled(new Set([Promise.resolve(1), Promise.reject("failure"), 3])), [
    { status: "fulfilled", value: 1 },
    { status: "rejected", reason: "failure" },
    { status: "fulfilled", value: 3 },
  ]);
  assertEquals(await allSettled([]), []);
});
