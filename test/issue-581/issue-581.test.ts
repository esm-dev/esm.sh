import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import dayjs from "http://localhost:8080/dayjs@1.11.7";
import relativeTime from "http://localhost:8080/dayjs@1.11.7/plugin/relativeTime.js";

dayjs.extend(relativeTime);

Deno.test("issue #581", async () => {
  assertEquals(dayjs(Date.now() - 3 * 1000).fromNow(), "a few seconds ago");
});
