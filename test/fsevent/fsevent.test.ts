import { assert } from "https://deno.land/std@0.178.0/testing/asserts.ts";

import fsevents from "http://localhost:8080/fsevents@2.3.2";

Deno.test("fsevent", () => {
  const stop = fsevents.watch(".", (path, flags, id) => {
    const info = fsevents.getInfo(path, flags, id);
    console.log(info);
  }); // To start observation
  stop(); // To end observation
});
