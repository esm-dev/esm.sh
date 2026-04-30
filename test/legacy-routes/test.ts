import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("legacy deprecated routes", async () => {
  try {
    await import("http://localhost:8080/");
  } catch (err: any) {
    assertStringIncludes(err.message, "deprecated");
  }
  try {
    await import("http://localhost:8080/v135");
  } catch (err: any) {
    assertStringIncludes(err.message, "deprecated");
  }
  {
    const { esm, build, transform } = await import("http://localhost:8080/build");
    assertEquals(typeof esm, "function");
    assertEquals(typeof build, "function");
    assertEquals(typeof transform, "function");
    try {
      esm``;
    } catch (err: any) {
      assertStringIncludes(err.message, "deprecated");
    }
  }
  {
    const { esm, build, transform } = await import("http://localhost:8080/v135/build");
    assertEquals(typeof esm, "function");
    assertEquals(typeof build, "function");
    assertEquals(typeof transform, "function");
    try {
      esm`let i: number = 0;`;
    } catch (err: any) {
      assertStringIncludes(err.message, "deprecated");
    }
  }
});

Deno.test("legacy routes (cache hit)", async () => {
  // fake cache
  await writeTextFile(
    ".esmd/storage/legacy/v135/react@19.0.0.meta",
    JSON.stringify({
      "esmId": "stable/react@19.0.0/es2022/react.mjs",
      "dts": "/v135/@types/react@latest/index.d.ts",
      "code":
        '/* esm.sh - react@19.0.0 */\nexport * from "/stable/react@19.0.0/es2022/react.mjs";\nexport { default } from "/stable/react@19.0.0/es2022/react.mjs";\n',
    }),
  );
  await writeTextFile(
    ".esmd/storage/legacy/react-dom@19.2.5.y35WJGFJWuY.meta",
    JSON.stringify({
      "esmId": "v135/react-dom@19.2.5/X-ZS9yZWFjdA/es2022/react-dom.mjs",
      "dts": "/v135/@types/react-dom@~19.2/X-ZS9yZWFjdA/index.d.ts",
      "code":
        '/* esm.sh - react-dom@19.2.5 */\nexport * from "/v135/react-dom@19.2.5/X-ZS9yZWFjdA/es2022/react-dom.mjs";\nexport { default } from "/v135/react-dom@19.2.5/X-ZS9yZWFjdA/es2022/react-dom.mjs";\n',
    }),
  );
  await writeTextFile(".esmd/storage/legacy/v135/react@19.2.5/es2022/react.js", "export const version = '19.2.5';");
  await writeTextFile(".esmd/storage/legacy/v135/@types/react@19.2.5/index.d.ts", "export const version:string;");

  {
    const res = await fetch("http://localhost:8080/v135/react@19.0.0", {
      redirect: "manual",
      headers: { "User-Agent": "i'm a browser" },
    });
    const text = await res.text();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("x-esm-id"), "stable/react@19.0.0/es2022/react.mjs");
    assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/v135/@types/react@latest/index.d.ts");
    assertEquals(
      text,
      '/* esm.sh - react@19.0.0 */\nexport * from "/stable/react@19.0.0/es2022/react.mjs";\nexport { default } from "/stable/react@19.0.0/es2022/react.mjs";\n',
    );
  }

  {
    const res = await fetch("http://localhost:8080/react-dom@19.2.5?pin=v135&target=2018&external=react", {
      redirect: "manual",
      headers: { "User-Agent": "i'm a browser" },
    });
    const text = await res.text();
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("x-esm-id"), "v135/react-dom@19.2.5/X-ZS9yZWFjdA/es2022/react-dom.mjs");
    assertEquals(res.headers.get("x-typescript-types"), "http://localhost:8080/v135/@types/react-dom@~19.2/X-ZS9yZWFjdA/index.d.ts");
    assertEquals(text, '/* esm.sh - react-dom@19.2.5 */\nexport * from "/v135/react-dom@19.2.5/X-ZS9yZWFjdA/es2022/react-dom.mjs";\nexport { default } from "/v135/react-dom@19.2.5/X-ZS9yZWFjdA/es2022/react-dom.mjs";\n');
  }

  {
    const res = await fetch("http://localhost:8080/v135/react@19.2.5/es2022/react.js", {
      redirect: "manual",
    });
    const text = await res.text();
    assertEquals(res.status, 200);
    assertEquals(text, "export const version = '19.2.5';");
  }

  {
    const res = await fetch("http://localhost:8080/v135/@types/react@19.2.5/index.d.ts", {
      redirect: "manual",
    });
    const text = await res.text();
    assertEquals(res.status, 200);
    assertEquals(text, "export const version:string;");
  }
});

Deno.test("legacy routes (cache miss)", async () => {
  {
    const res = await fetch("http://localhost:8080/stable/react@18.3.1", {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 301);
    assert(res.headers.get("Location")?.startsWith("http://localhost:8080/react@18.3.1"));
  }
  {
    const res = await fetch("http://localhost:8080/v135/react@18.3.1", {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 301);
    assert(res.headers.get("Location")?.startsWith("http://localhost:8080/react@18.3.1"));
  }
  {
    const res = await fetch("http://localhost:8080/v135/react@18.3.1?target=2018", {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 301);
    assert(res.headers.get("Location")?.startsWith("http://localhost:8080/react@18.3.1?target=2018"));
  }
  {
    const res = await fetch("http://localhost:8080/stable/react", {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assert(res.headers.get("Location")?.startsWith("http://localhost:8080/stable/react@"));
  }
  {
    const res = await fetch("http://localhost:8080/v135/react@18", {
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 302);
    assert(res.headers.get("Location")?.startsWith("http://localhost:8080/v135/react@18."));
  }
  {
    const res = await fetch("http://localhost:8080/v135/node_process.js", {
      headers: { "User-Agent": "i'm a browser" },
      redirect: "manual",
    });
    res.body?.cancel();
    assertEquals(res.status, 301);
    assert(res.headers.get("Location")?.startsWith("http://localhost:8080/node/process.mjs"));
  }
  {
    const res = await fetch("http://localhost:8080/v135/node.ns.d.ts", {
      headers: { "User-Agent": "Deno/1.42.0" },
    });
    await res.body?.cancel();
    assertEquals(res.status, 404);
  }
  {
    // invalid build version
    const res = await fetch("http://localhost:8080/v136/react-dom@18.3.1/es2022/client.js", {
      headers: { "User-Agent": "i'm a browser" },
    });
    await res.body?.cancel();
    assertEquals(res.status, 400);
  }
});

async function writeTextFile(path: string, content: string) {
  const dir = path.split("/").slice(0, -1).join("/");
  await Deno.mkdir(dir, { recursive: true });
  await Deno.writeTextFile(path, content);
}
