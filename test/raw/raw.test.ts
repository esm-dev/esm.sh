import {
    assertEquals,
    assertStringIncludes,
  } from "https://deno.land/std@0.180.0/testing/asserts.ts";
  
  Deno.test("raw untransformed JS", async () => {
    const res = await fetch(
      "http://localhost:8080/playground-elements@0.18.1/playground-service-worker.js?raw",
    );
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("content-type"), "application/javascript");
    assertStringIncludes(await res.text(), "!function(){");
  });
  