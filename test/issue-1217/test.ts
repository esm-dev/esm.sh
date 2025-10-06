import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #1217 - CSS files should resolve through package.json exports", async () => {
  // Test case 1: yet-another-react-lightbox
  // The package.json has: "./styles.css": {"default": "./dist/styles.css"}
  // Should redirect from /styles.css to /dist/styles.css
  const res1 = await fetch(
    "https://esm.sh/*yet-another-react-lightbox@3.21.7/styles.css",
    { redirect: "follow" },
  );
  const css1 = await res1.text();
  assertEquals(res1.ok, true, "yet-another-react-lightbox styles.css should be found");
  assertEquals(res1.status, 200, "Should return 200 OK");
  assertStringIncludes(res1.url, "/dist/styles.css", "Should resolve to /dist/styles.css");
  assertStringIncludes(css1, "yarl__", "Should contain lightbox CSS");

  // Test case 2: react-tweet
  // The package.json has: "./theme.css": "./dist/twitter-theme/theme.css"
  // Should redirect from /theme.css to /dist/twitter-theme/theme.css
  const res2 = await fetch(
    "https://esm.sh/*react-tweet@3.2.2/theme.css",
    { redirect: "follow" },
  );
  const css2 = await res2.text();
  assertEquals(res2.ok, true, "react-tweet theme.css should be found");
  assertEquals(res2.status, 200, "Should return 200 OK");
  assertStringIncludes(res2.url, "/dist/twitter-theme/theme.css", "Should resolve to /dist/twitter-theme/theme.css");
  assertStringIncludes(css2, "tweet", "Should contain tweet CSS");
});
