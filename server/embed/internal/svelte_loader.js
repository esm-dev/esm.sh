import { compile, VERSION } from "svelte/compiler";

const majorVersion = Number(VERSION.split(".")[0]);
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
  if (majorVersion >= 5) {
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
