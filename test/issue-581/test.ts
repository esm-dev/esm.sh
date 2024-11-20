import { assertEquals } from "jsr:@std/assert";

import dayjs from "http://localhost:8080/dayjs@1.11.7";
import relativeTime from "http://localhost:8080/dayjs@1.11.7/plugin/relativeTime.mjs";

dayjs.extend(relativeTime);

Deno.test("issue #581", async () => {
  assertEquals(dayjs(Date.now() - 3 * 1000).fromNow(), "a few seconds ago");
});
