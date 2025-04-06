import { TextLineStream } from "jsr:@std/streams@1.0.9/text-line-stream";

const enc = new TextEncoder();
const output = (type, data) => Deno.stdout.write(enc.encode(">>>" + type + ":" + JSON.stringify(data) + "\n"));

for await (const line of Deno.stdin.readable.pipeThrough(new TextDecoderStream()).pipeThrough(new TextLineStream())) {
  try {
    const [filename, fn, args] = JSON.parse(line);
    const modUrl = new URL(filename, "file://" + Deno.cwd() + "/").href;
    const mod = await import(modUrl);
    const f = mod[fn];
    if (typeof f !== "function") {
      throw new Error(`function ${fn} not found in module ${filename}`);
    }
    const ret = await f(...args);
    output("json", ret);
  } catch (e) {
    output("error", e.message);
  }
}
