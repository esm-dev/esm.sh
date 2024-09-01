import { assert, assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { h } from "preact";
import render from "preact-render-to-string";
import useSWR from "swr";

Deno.test("external all", () => {
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
  assert(html == "<main><p>just now</p></main>");
});

Deno.test("external all 2", async () => {
  const res = await fetch("http://localhost:8080/*preact@10.23.2/jsx-runtime");
  const code = await res.text();
  assertStringIncludes(code, "preact@10.23.2/X-ZS8q/");
});
