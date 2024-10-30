import { compile, VERSION } from "svelte/compiler";

const version = Number(VERSION.split(".")[0]);
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

try {
  const [filename, code] = JSON.parse(await readStdin());
  const options = { filename, css: "injected" };
  if (version >= 5) {
    options.runes = true;
    options.generate = "client";
  } else {
    options.generate = "dom";
  }
  const { js } = compile(code, options);
  stdout.write(JSON.stringify({ code: js.code }));
} catch (err) {
  stdout.write(JSON.stringify({ error: err.message }));
}

process.exit(0);
