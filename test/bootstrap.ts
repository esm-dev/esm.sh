#!/usr/bin/env -S deno run --allow-run --allow-read --allow-write --allow-net

async function startServer(onStart: () => Promise<void>, verbose: boolean) {
  const { code, success } = await run("go", "build", "-o", "esmd", "main.go");
  if (!success) {
    Deno.exit(code);
  }
  const p = new Deno.Command("./esmd", {
    stdout: verbose ? "inherit" : "null",
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
        onStart();
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
  const tests = Deno.args.filter((arg) => !arg.startsWith("-"));
  const clean = Deno.args.includes("--clean");
  if (clean) {
    try {
      console.log("Cleaning up...");
      await Promise.all([
        Deno.remove("./.esmd/log", { recursive: true }),
        Deno.remove("./.esmd/storage", { recursive: true }),
        Deno.remove("./.esmd/esm.db"),
      ]);
    } catch (_) {
      // ignore
    }
  }
  console.log("Starting esm.sh server...");
  startServer(async () => {
    let timeUsed = 0;
    if (tests.length > 0) {
      for (const testDir of tests) {
        timeUsed += await runTest(testDir, true);
      }
    } else {
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
  }, tests.length > 0);
}
