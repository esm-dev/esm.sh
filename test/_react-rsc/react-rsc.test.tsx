// deno-lint-ignore-file no-explicit-any
import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";
// deno-lint-ignore ban-ts-comment
// @ts-ignore
globalThis.__webpack_require__ = () => {};
import React from "http://localhost:8080/react@canary?conditions=react-server";
import ReactDom from "http://localhost:8080/react-dom@canary/server?conditions=react-server";
import ReactServerDom from "http://localhost:8080/react-server-dom-turbopack@canary/server?deps=react@canary,react-dom@canary&conditions=react-server";

/**
 * Wrap a client-side module import with metadata
 * that tells React this is a client-side component.
 * @param {string} id Client-side component ID. Used to look up React metadata.
 * @param {string} localImportPath Path to client-side module on the file system.
 */
function getClientComponentModule(id: string, localImportPath: string) {
  return `
import DefaultExport from ${JSON.stringify(localImportPath)}
DefaultExport.$$typeof = Symbol.for("react.client.reference")
DefaultExport.$$id=${JSON.stringify(id)}
export default DefaultExport
`;
}

const FooCode = `
"use client"
import React from "http://localhost:8080/react@canary"
export default () => React.createElement("h2", null, "Foo")
`;

Deno.test("react-rsc", async () => {
  const Foo = (await import(
    `data:text/javascript,${
      encodeURIComponent(
        getClientComponentModule(
          "Foo",
          `data:text/javascript,${encodeURIComponent(FooCode)}`,
        ),
      )
    }`
  )).default;
  const Albums = (async () => {
    await new Promise((resolve) => setTimeout(resolve, 200));
    return (
      <>
        <Foo />
        <ul>
          <li>Post</li>
          <li>The Fame</li>
          <li>How To Be A Human Being</li>
        </ul>
      </>
    );
  }) as any;
  const Suspense = React.Suspense as any;
  const App = () => (
    <>
      <h1>AbraMix</h1>
      <Suspense fallback={<p>Loading...</p>}>
        <Albums />
      </Suspense>
    </>
  );
  const res = new Response(
    await ReactServerDom.renderToReadableStream(
      <App />,
      {
        "Foo": {
          "id": "Foo",
          "chunks": [],
          "name": "default",
          "async": true,
        },
      },
    ),
    { headers: { "Content-type": "text/x-component" } },
  );
  const res2 = new Response(
    await ReactDom.renderToReadableStream(
      <App />,
    ),
    { headers: { "Content-type": "text/html" } },
  );

  const chunks = await res.text();
  console.log(chunks);
  assertStringIncludes(chunks, `{"children":"AbraMix"}`);
  assertStringIncludes(chunks, `{"children":"Loading..."}`);
  assertStringIncludes(chunks, `"id":"Foo"`);
  assertStringIncludes(chunks, `{"children":"Post"}`);
  assertStringIncludes(chunks, `{"children":"The Fame"}`);
  assertStringIncludes(chunks, `{"children":"How To Be A Human Being"}`);

  const html = await res2.text();
  console.log(html);
  assertStringIncludes(html, `<h1>AbraMix</h1>`);
  assertStringIncludes(html, `<p>Loading...</p>`);
  assertStringIncludes(html, `<h2>Foo</h2>`);
  assertStringIncludes(html, `<li>Post</li>`);
  assertStringIncludes(html, `<li>The Fame</li>`);
  assertStringIncludes(html, `<li>How To Be A Human Being</li>`);
});
