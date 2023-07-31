#!/usr/bin/env -S deno run --allow-run --allow-read --allow-write --allow-net

async function startServer(onStart: () => Promise<void>, single: boolean) {
  const { code, success } = await run("go", "build", "-o", "esmd", "main.go");
  if (!success) {
    Deno.exit(code);
  }
  const p = new Deno.Command("./esmd", {
    stdout: single ? "inherit" : "null",
    stderr: "inherit",
  }).spawn();
  addEventListener("unload", () => {
    console.log("%cClosing esm.sh server...", "color: grey");
    p.kill("SIGINT");
  });
  while (true) {
    try {
      await new Promise((resolve) => setTimeout(resolve, 100));
      const res = await fetch(`http://localhost:8080/status.json`);
      const status = await res.json();
      if (status.ns === "READY") {
        console.log("esm.sh server started.");
        await onStart();
        break;
      }
    } catch (_) {
      // ignore
    }
  }
  await p.status;
}

async function runTest(name: string, retry?: boolean): Promise<number> {
  const execBegin = Date.now();
  const args = [
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
    args.push("--config", dir + "deno.json");
  }
  args.push(dir);

  console.log(`\n[test ${name}]`);

  const { code, success } = await run(Deno.execPath(), ...args);
  if (!success) {
    if (!retry) {
      console.log("something wrong, retry...");
      await new Promise((resolve) => setTimeout(resolve, 500));
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

  const p = new Deno.Command(Deno.execPath(), {
    args: [
      "run",
      "-A",
      "-r",
      "--no-lock",
      "http://localhost:8080/v100/cli",
      "add",
      "preact@10.10.6",
      "preact-render-to-string@5.2.3",
      "react:preact@10.10.6/compat",
      "swr@1.3.0",
    ],
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  }).spawn();
  const { code, success } = await p.status;
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

function run(name: string, ...args: string[]) {
  const p = new Deno.Command(name, {
    args,
    stdout: "inherit",
    stderr: "inherit",
  }).spawn();
  return p.status;
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
  const rootDir = new URL(import.meta.url).pathname.split("/").slice(0, -2)
    .join("/");
  Deno.chdir(rootDir);
  const [testDir] = Deno.args.filter((arg) => !arg.startsWith("-"));
  const clean = Deno.args.includes("--clean");
  if (clean) {
    console.log("Cleaning up...");
    try {
      await Deno.remove("./.esmd/log", { recursive: true });
      await Deno.remove("./.esmd/storage", { recursive: true });
      await Deno.remove("./.esmd/esm.db");
    } catch (_) {
      // ignore
    }
  }
  console.log("Starting esm.sh server...");
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
