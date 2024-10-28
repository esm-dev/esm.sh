import { transform } from "esm-vue-sfc-compiler";
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
  const [filename, srouceCode] = JSON.parse(await readStdin());
  const code = await transform(filename, srouceCode);
  stdout.write(JSON.stringify({ code }));
} catch (err) {
  stdout.write(JSON.stringify({ error: err.message, stack: err.stack }));
}

process.exit(0);
