import {
  assert,
  assertEquals,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("CLI script", async () => {
  const res = await fetch("http://localhost:8080/");
  await res.body?.cancel();
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type"),
    "application/typescript; charset=utf-8",
  );
});

Deno.test("Using deno.json as import maps", async () => {
  const cwd = await Deno.makeTempDir();
  const denoJson = cwd + "/deno.json";
  await Deno.writeTextFile(
    denoJson,
    `{"tasks": {"test": "deno test"}}`,
  );

  const p0 = new Deno.Command(Deno.execPath(), {
    args: [
      "run",
      "-A",
      "-r",
      "--no-lock",
      "http://localhost:8080/v100",
      "init",
    ],
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  }).spawn();
  assert((await p0.status).success);

  const denoConfigRaw = await Deno.readTextFile(denoJson);
  const denoConfig = JSON.parse(denoConfigRaw);
  assertEquals(denoConfig.tasks, {
    "test": "deno test",
    "esm:add": `deno run -A http://localhost:8080/v100 add`,
    "esm:update": `deno run -A http://localhost:8080/v100 update`,
    "esm:remove": `deno run -A http://localhost:8080/v100 remove`,
  });

  const p = new Deno.Command(Deno.execPath(), {
    args: [
      "task",
      "esm:add",
      "preact@10.10.6",
      "preact-render-to-string@5.2.3",
      "react:preact@10.10.6/compat",
      "swr@1.3.0",
    ],
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  }).spawn();
  const { success } = await p.status;
  assert(success);

  const imRaw = await Deno.readTextFile(denoJson);
  const im = JSON.parse(imRaw);
  assertEquals(im.imports, {
    "preact-render-to-string":
      "http://localhost:8080/v100/*preact-render-to-string@5.2.3",
    "preact-render-to-string/":
      "http://localhost:8080/v100/*preact-render-to-string@5.2.3/",
    preact: "http://localhost:8080/v100/preact@10.10.6",
    "preact/": "http://localhost:8080/v100/preact@10.10.6/",
    react: "http://localhost:8080/v100/preact@10.10.6/compat",
    swr: "http://localhost:8080/v100/*swr@1.3.0",
    "swr/": "http://localhost:8080/v100/*swr@1.3.0/",
  });
  assertEquals(im.scopes, {
    "http://localhost:8080/v100/": {
      "pretty-format": "http://localhost:8080/v100/pretty-format@3.8.0",
    },
  });
});

Deno.test("Using external import maps", async () => {
  const cwd = await Deno.makeTempDir();
  const denoJson = cwd + "/deno.json";
  await Deno.writeTextFile(
    denoJson,
    `{"importMap": "import-map.json", "tasks": {"test": "deno test"}}`,
  );

  const p0 = new Deno.Command(Deno.execPath(), {
    args: [
      "run",
      "-A",
      "-r",
      "--no-lock",
      "http://localhost:8080/v100",
      "init",
    ],
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  }).spawn();
  assert((await p0.status).success);

  const denoConfigRaw = await Deno.readTextFile(denoJson);
  const denoConfig = JSON.parse(denoConfigRaw);
  assertEquals(denoConfig.tasks, {
    "test": "deno test",
    "esm:add": `deno run -A http://localhost:8080/v100 add`,
    "esm:update": `deno run -A http://localhost:8080/v100 update`,
    "esm:remove": `deno run -A http://localhost:8080/v100 remove`,
  });

  const p = new Deno.Command(Deno.execPath(), {
    args: [
      "task",
      "esm:add",
      "preact@10.10.6",
      "preact-render-to-string@5.2.3",
      "react:preact@10.10.6/compat",
      "swr@1.3.0",
    ],
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  }).spawn();
  const { success } = await p.status;
  assert(success);

  const imRaw = await Deno.readTextFile(cwd + "/import-map.json");
  const im = JSON.parse(imRaw);
  assertEquals(im, {
    imports: {
      "preact-render-to-string":
        "http://localhost:8080/v100/*preact-render-to-string@5.2.3",
      "preact-render-to-string/":
        "http://localhost:8080/v100/*preact-render-to-string@5.2.3/",
      preact: "http://localhost:8080/v100/preact@10.10.6",
      "preact/": "http://localhost:8080/v100/preact@10.10.6/",
      react: "http://localhost:8080/v100/preact@10.10.6/compat",
      swr: "http://localhost:8080/v100/*swr@1.3.0",
      "swr/": "http://localhost:8080/v100/*swr@1.3.0/",
    },
    scopes: {
      "http://localhost:8080/v100/": {
        "pretty-format": "http://localhost:8080/v100/pretty-format@3.8.0",
      },
    },
  });
});
