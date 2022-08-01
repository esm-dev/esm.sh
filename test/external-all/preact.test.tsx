import { assert } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import { h } from "preact";
import render from "preact-render-to-string";
import useSWR from "swr";

Deno.test("?external=*", async () => {
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
