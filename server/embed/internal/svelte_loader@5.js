import { compile } from "svelte/compiler";
const { stdin, stdout } = process;

function readStdin() {
  return new Promise((resolve) => {
    let buf = "";
    stdin.setEncoding("utf8");
    stdin.on("data", (chunk) => {
      buf += chunk;
    });
    stdin.on("end", () => resolve(buf));
  });
}

async function load() {
  try {
    const [filename, code] = JSON.parse(await readStdin());
    const { js } = compile(code, { filename, generate: "client", runes: true, css: "injected" });
    stdout.write(JSON.stringify({ code: js.code }));
  } catch (err) {
    stdout.write(JSON.stringify({ error: err.message, stack: err.stack }));
  }

  process.exit(0);
}

load();
