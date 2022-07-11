import { assert } from "https://deno.land/std@0.145.0/testing/asserts.ts";

import { h } from "http://localhost:8080/preact@10.7.2";
import render from "http://localhost:8080/preact-render-to-string@5.2.0?deps=preact@10.7.2";
import useSWR from "http://localhost:8080/swr@1.3.0?alias=react:preact/compat&deps=preact@10.7.2";

Deno.test("preact-swr(external)", async () => {
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
