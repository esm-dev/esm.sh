import { assertEquals } from "jsr:@std/assert";

for (const target of ["es2022", "deno", "denonext"]) {
  Deno.test(`issue #739: Expo sends notifications (${target})`, async () => {
    const fetch = globalThis.fetch;
    const messages = [{ to: "ExponentPushToken[test-token]", body: "test" }];
    const tickets = [{ status: "ok", id: "test-ticket" }];
    let requests = 0;
    globalThis.fetch = (input, init?: RequestInit) => {
      requests++;
      assertEquals(input, "https://exp.host/--/api/v2/push/send");
      assertEquals(init?.method, "post");
      assertEquals(
        new Headers(init?.headers).get("content-type"),
        "application/json",
      );
      assertEquals(init?.body, JSON.stringify(messages));
      return Promise.resolve(Response.json({ data: tickets }));
    };
    try {
      const { Expo } = await import(
        `http://localhost:8080/expo-server-sdk@3.7.0?target=${target}`
      );
      assertEquals(
        await new Expo().sendPushNotificationsAsync(messages),
        tickets,
      );
      assertEquals(requests, 1);
    } finally {
      globalThis.fetch = fetch;
    }
  });
}
