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

try {
  const [filename, code] = JSON.parse(await readStdin());
  const { js } = compile(code, { filename, css: "injected" });
  stdout.write(JSON.stringify({ code: js.code }));
} catch (err) {
  stdout.write(JSON.stringify({ error: err.message }));
}

process.exit(0);
