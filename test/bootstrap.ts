const [testDir] = Deno.args;

startEsmServer(async () => {
  console.log("esm.sh server started");
  if (testDir) {
    await runTest(testDir, true);
  } else {
    for await (const entry of Deno.readDir("./test")) {
      if (entry.isDirectory) {
        await runTest(entry.name);
      }
    }
  }
});

async function startEsmServer(onReady: () => void) {
  await run("go", "build", "-o", "esmd", "main.go");
  const p = Deno.run({
    cmd: ["./esmd", "--port", "8080"],
    stdout: "null",
    stderr: "inherit",
  });
  globalThis.addEventListener("unload", (e) => {
    p.kill("SIGINT");
  });
  while (true) {
    try {
      await new Promise((resolve) => setTimeout(resolve, 500));
      const body = await fetch(`http://localhost:8088`).then((res) =>
        res.text()
      );
      if (body === "READY") {
        onReady();
        break;
      }
    } catch (_) {}
  }
  await p.status();
}

async function runTest(name: string, retry?: boolean) {
  const cmd = [
    Deno.execPath(),
    "test",
    "-A",
    "--check=all",
    "--unstable",
    "--reload=http://localhost:8080",
    "--location=http://0.0.0.0/",
  ];
  const dir = `test/${name}/`;
  if (await existsFile(dir + "deno.json")) {
    cmd.push("--config", dir + "deno.json");
  }
  cmd.push(dir);

  console.log(`\n[testing ${name}]`);

  const { code, success } = await run(...cmd);
  if (!success && !retry) {
    console.log("something wrong, retry...");
    await new Promise((resolve) => setTimeout(resolve, 100));
    await runTest(name, true);
    return;
  }
  console.log("Done!");
  Deno.exit(code);
}

async function run(...cmd: string[]) {
  return await Deno.run({ cmd, stdout: "inherit", stderr: "inherit" }).status();
}

/* check whether or not the given path exists as regular file. */
export async function existsFile(path: string): Promise<boolean> {
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
