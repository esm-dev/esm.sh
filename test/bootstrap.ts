#!/usr/bin/env -S deno run --allow-run --allow-read --allow-write --allow-net

async function startServer(onStart: () => void, single: boolean) {
  await run("go", "build", "-o", "esmd", "main.go");
  const p = Deno.run({
    cmd: ["./esmd"],
    stdout: single ? "inherit" : "null",
    stderr: "inherit",
  });
  addEventListener("unload", (e) => {
    console.log("%cClosing esm.sh server...", "color: grey");
    p.kill("SIGINT");
  });
  while (true) {
    try {
      await new Promise((resolve) => setTimeout(resolve, 500));
      const body = await fetch(`http://localhost:8088`).then((res) =>
        res.text()
      );
      if (body === "READY") {
        console.log("esm.sh server started.");
        onStart();
        break;
      }
    } catch (_) {}
  }
  await p.status();
}

async function runTest(name: string, retry?: boolean): Promise<number> {
  const execBegin = Date.now();
  const cmd = [
    Deno.execPath(),
    "test",
    "-A",
    "--unstable",
    "--check",
    "--no-lock",
    "--reload=http://localhost:8080",
    "--location=http://0.0.0.0/",
  ];
  const dir = `test/${name}/`;
  if (await existsFile(dir + "deno.json")) {
    cmd.push("--config", dir + "deno.json");
  }
  cmd.push(dir);

  console.log(`\n[test ${name}]`);

  const { code, success } = await run(...cmd);
  if (!success) {
    if (!retry) {
      console.log("something wrong, retry...");
      await new Promise((resolve) => setTimeout(resolve, 100));
      return await runTest(name, true);
    } else {
      Deno.exit(code);
    }
  }
  return Date.now() - execBegin;
}

async function runCliTest() {
  console.log(`\n[test CLI]`);

  const cwd = await Deno.makeTempDir();
  await Deno.writeTextFile(
    cwd + "/deno.json",
    `{"importMap": "import-map.json"}`,
  );

  const res = await fetch("http://localhost:8080/");
  if (!res.headers.get("content-type")?.startsWith("application/typescript")) {
    throw new Error(`Invalid content type: ${res.headers.get("content-type")}`);
  }

  const { code, success } = await Deno.run({
    cmd: [
      Deno.execPath(),
      "run",
      "-A",
      "-r",
      "--no-lock",
      "http://localhost:8080/v100",
      "add",
      "preact@10.10.6",
      "preact-render-to-string@5.2.3",
      "react:preact@10.10.6/compat",
      "swr@1.3.0",
    ],
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  }).status();
  if (!success) {
    Deno.exit(code);
  }

  const imRaw = await Deno.readTextFile(cwd + "/import-map.json");
  const im = JSON.parse(imRaw);
  if (
    JSON.stringify(im) !== JSON.stringify({
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
    })
  ) {
    console.log(im);
    throw new Error("Invalid import maps generated");
  }
}

async function run(...cmd: string[]) {
  return await Deno.run({ cmd, stdout: "inherit", stderr: "inherit" }).status();
}

async function existsFile(path: string): Promise<boolean> {
  try {
    const fi = await Deno.lstat(path);
    return fi.isFile;
  } catch (err) {
    if (err instanceof Deno.errors.NotFound) {
      return false;
    }
    throw err;
  }
}

if (import.meta.main) {
  const [testDir] = Deno.args;
  startServer(async () => {
    let timeUsed = 0;
    if (testDir) {
      timeUsed += await runTest(testDir, true);
    } else {
      await runCliTest();
      for await (const entry of Deno.readDir("./test")) {
        if (entry.isDirectory && !entry.name.startsWith("_")) {
          timeUsed += await runTest(entry.name);
        }
      }
    }
    timeUsed = Math.ceil(timeUsed / 1000);
    console.log(
      `Done! Total time spent: %c${Math.floor(timeUsed / 60)}m${
        timeUsed % 60
      }s`,
      "color: blue",
    );
    Deno.exit(0);
  }, Boolean(testDir));
}
