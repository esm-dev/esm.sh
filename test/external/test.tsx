import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

import { h } from "preact";
import render from "preact-render-to-string";
import useSWR from "swr";

Deno.test("external", () => {
  const fetcher = (url: string) => fetch(url).then((res) => res.json());
  const App = () => {
    const { data } = useSWR("http://localhost:8080/status.json", fetcher, {
      fallbackData: { uptime: "just now" },
    });
    if (!data) {
      return (
        <main>
          <p>loading...</p>
        </main>
      );
    }
    return (
      <main>
        <p>{data.uptime}</p>
      </main>
    );
  };
  const html = render(<App />);
  assertEquals(html, "<main><p>just now</p></main>");
});

Deno.test("types with ?external", async () => {
  const res = await fetch(`http://localhost:8080/swr@1.3.0/X-ZXJlYWN0/dist/use-swr.d.ts`);
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("Content-type"), "application/typescript; charset=utf-8");
  const ts = await res.text();
  assertStringIncludes(ts, '/// <reference types="react" />');
  assertStringIncludes(ts, 'import("react")');
});
