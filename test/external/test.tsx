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

Deno.test("strip invalid ?external", async () => {
  const res1 = await fetch("http://localhost:8080/react-dom@18.3.1?target=es2022&external=foo,bar,react");
  const code1 = await res1.text();
  assertStringIncludes(code1, '"/react-dom@18.3.1/X-ZXJlYWN0/es2022/react-dom.mjs"');

  const res2 = await fetch("http://localhost:8080/react-dom@18.3.1?target=es2022&external=foo,bar,preact");
  const code2 = await res2.text();
  assertStringIncludes(code2, '"/react-dom@18.3.1/es2022/react-dom.mjs"');
});

Deno.test("types with ?external", async () => {
  const res = await fetch(`http://localhost:8080/swr@1.3.0/X-ZXJlYWN0/dist/use-swr.d.ts`);
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("Content-type"), "application/typescript; charset=utf-8");
  const ts = await res.text();
  assertStringIncludes(ts, '/// <reference types="react" />');
  assertStringIncludes(ts, 'import("react")');
});
