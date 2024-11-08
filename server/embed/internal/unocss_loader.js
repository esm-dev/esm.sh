import { generate } from "@esm.sh/unocss";
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
  const [configCSS, content] = JSON.parse(await readStdin());
  const code = await generate(content, configCSS ? { configCSS } : undefined);
  stdout.write(JSON.stringify({ code }));
} catch (err) {
  stdout.write(JSON.stringify({ error: err.message }));
}

process.exit(0);
