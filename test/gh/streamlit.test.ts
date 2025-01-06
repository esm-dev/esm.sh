import { assertEquals } from "jsr:@std/assert";

import { Streamlit } from "http://localhost:8080/gh/streamlit/streamlit@1.34.0/component-lib";

Deno.test("streamlit from github", async () => {
  assertEquals(typeof Streamlit, "function");
});
