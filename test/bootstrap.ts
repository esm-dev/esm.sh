const [testDir] = Deno.args;

startEsmServer(async () => {
  console.log("esm.sh server started.");
  await runCli();
  if (testDir) {
    await runTest(testDir, true);
  } else {
    for await (const entry of Deno.readDir("./test")) {
      if (entry.isDirectory) {
        await runTest(entry.name);
      }
    }
  }
  console.log("Done!");
  Deno.exit(0);
});

async function startEsmServer(onReady: () => void) {
  await run("go", "build", "-o", "esmd", "main.go");
  const p = Deno.run({
    cmd: ["./esmd", "--port", "8080"],
    stdout: "null",
    stderr: "inherit",
  });
  addEventListener("unload", (e) => {
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

  console.log(`\n[test ${name}]`);

  const { code, success } = await run(...cmd);
  if (!success) {
    if (!retry) {
      console.log("something wrong, retry...");
      await new Promise((resolve) => setTimeout(resolve, 100));
      await runTest(name, true);
    } else {
      Deno.exit(code);
    }
  }
}

async function runCli() {
  const cmd = [
    Deno.execPath(),
    "run",
    "-A",
    "-r",
    "http://localhost:8080/v94",
    "add",
    "react@18.2.0",
    "react-dom@18.2.0",
    "swr@1.3.0",
  ];
  console.log(`\n[test CLI]`);

  const cwd = await Deno.makeTempDir();
  const { code, success } = await Deno.run({
    cmd,
    cwd,
    stdout: "inherit",
    stderr: "inherit",
  }).status();
  if (!success) {
    Deno.exit(code);
  }
  const imRaw = await Deno.readTextFile(cwd + "/import_map.json");
  const im = JSON.parse(imRaw);
  if (
    im.imports["react-dom"] !==
      "http://localhost:8080/v94/react-dom@18.2.0&external=react" ||
    im.imports["react"] !== "http://localhost:8080/v94/react@18.2.0" ||
    im.imports["swr"] !== "http://localhost:8080/v94/swr@1.3.0&external=react"
  ) {
    console.log(im);
    throw new Error("Invalid import maps generated");
  }
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
