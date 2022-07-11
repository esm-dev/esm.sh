const [testDir] = Deno.args;

startEsmServer(async (p) => {
  console.log("esm.sh server started");
  try {
    if (testDir) {
      await runTest(testDir, p);
    } else {
      for await (const entry of Deno.readDir("test/deno")) {
        if (entry.isDirectory) {
          await runTest(entry.name, p);
        }
      }
    }
    console.log("Done!");
  } catch (error) {
    console.error(error);
  }
  p.kill("SIGINT");
});

async function startEsmServer(onReady: (p: any) => void) {
  await run("go", "build", "-o", "esmd", "main.go");
  const p = Deno.run({
    cmd: ["./esmd", "-dev", "-port", "8080"],
    stdout: "piped",
    stderr: "inherit",
  });
  let output = "";
  const buf = new Uint8Array(32);
  for (let index = 0; index < 1000; index++) {
    const n = await p.stdout?.read(buf);
    if (!n) {
      break;
    }
    output += new TextDecoder().decode(buf.slice(0, n));
    if (output.includes("node services process started")) {
      onReady(p);
      break;
    }
  }
  await p.status();
}

async function runTest(name: string, p: any, retryTimes = 0) {
  const cmd = [
    Deno.execPath(),
    "test",
    "-A",
    "--unstable",
    "--reload=http://localhost:8080",
    "--location=http://0.0.0.0/",
  ];
  const dir = `test/deno/${name}/`;
  if (await existsFile(dir + "deno.json")) {
    cmd.push("--config", dir + "deno.json");
  }
  cmd.push(dir);

  console.log(`\n[testing ${name}]`);

  const { code, success } = await run(...cmd);
  if (!success) {
    if (retryTimes < 3) {
      console.log("something wrong, retry...");
      await new Promise((resolve) => setTimeout(resolve, 100));
      await runTest(name, p, retryTimes + 1);
    } else {
      p.kill("SIGINT");
      Deno.exit(code);
    }
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
