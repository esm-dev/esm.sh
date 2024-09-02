import { assertEquals } from "jsr:@std/assert";
import { h } from "http://localhost:8080/preact@10.7.2";
import { useState } from "http://localhost:8080/preact@10.14.0/hooks";
import render from "http://localhost:8080/preact-render-to-string@6.0.3?deps=preact@10.14.0";

Deno.test("preact-jsx", () => {
  const div = h("div", null, h("h1", null, "Hey"));
  const divx = (
    <div>
      <h1>Hey</h1>
    </div>
  );
  assertEquals(div.type, "div");
  assertEquals(div.props.children.type, "h1");
  assertEquals(div.props.children.props.children, "Hey");
  assertEquals(divx.type, "div");
  assertEquals(divx.props.children.type, "h1");
  assertEquals(divx.props.children.props.children, "Hey");
});

Deno.test("preact-render-to-string", () => {
  const App = () => {
    const [message] = useState("Hey");
    return (
      <main>
        <h1>{message}</h1>
      </main>
    );
  };
  const html = render(<App />);
  assertEquals(html, "<main><h1>Hey</h1></main>");
});
