import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import Pusher from "http://localhost:8080/pusher@5.1.2";

Deno.test("issue #649", async () => {
  const pusher = new Pusher({
    appId: "ESM_SH",
    key: "KEY",
    secret: "SECRET",
    host: "localhost",
    port: "8080",
  });
  try {
    await pusher.trigger("chat", "message", {
      message: ":)",
    });
  } catch (e) {
    assertEquals(e.status, 404);
  }
});
