import { assertEquals } from "jsr:@std/assert";

const badUrls = [
  "http://localhost:8080/wp-admin.php",
  "http://localhost:8080/.well-known/appspecific/com.chrome.devtools.json",
  "http://localhost:8080/.env",
  "http://localhost:8080/react@18.0.1/../../../../../../../../../../etc/passwd?raw=1&module=1",
];

Deno.test("ban bad urls", async () => {
  for (const url of badUrls) {
    const res = await fetch(url);
    await res.body?.cancel();
    assertEquals(res.status, 404, url);
  }
});
