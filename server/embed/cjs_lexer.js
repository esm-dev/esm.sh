const fs = require("fs");
const { dirname } = require("path");
const { promisify } = require("util");
const { parse } = require("esm-cjs-lexer");
const enhancedResolve = require("enhanced-resolve");

const identRegexp = /^[a-zA-Z_\$][\w\$]*$/;
const reservedWords = new Set([
  "abstract",
  "arguments",
  "await",
  "boolean",
  "break",
  "byte",
  "case",
  "catch",
  "char",
  "class",
  "const",
  "continue",
  "debugger",
  "default",
  "delete",
  "do",
  "double",
  "else",
  "enum",
  "eval",
  "export",
  "extends",
  "false",
  "final",
  "finally",
  "float",
  "for",
  "function",
  "goto",
  "if",
  "implements",
  "import",
  "in",
  "instanceof",
  "int",
  "interface",
  "let",
  "long",
  "native",
  "new",
  "null",
  "package",
  "private",
  "protected",
  "public",
  "return",
  "short",
  "static",
  "super",
  "switch",
  "synchronized",
  "this",
  "throw",
  "throws",
  "transient",
  "true",
  "try",
  "typeof",
  "var",
  "void",
  "volatile",
  "while",
  "with",
  "yield",
]);
const builtInNodeModules = new Set([
  "assert",
  "async_hooks",
  "child_process",
  "cluster",
  "buffer",
  "console",
  "constants",
  "crypto",
  "dgram",
  "dns",
  "domain",
  "events",
  "fs",
  "fs/promises",
  "http",
  "http2",
  "https",
  "inspector",
  "module",
  "net",
  "os",
  "path",
  "path/posix",
  "path/win32",
  "perf_hooks",
  "process",
  "punycode",
  "querystring",
  "readline",
  "repl",
  "stream",
  "stream/promises",
  "stream/web",
  "_stream_duplex",
  "_stream_passthrough",
  "_stream_readable",
  "_stream_transform",
  "_stream_writable",
  "string_decoder",
  "sys",
  "timers",
  "tls",
  "trace_events",
  "tty",
  "url",
  "util",
  "v8",
  "vm",
  "worker_threads",
  "zlib",
]);

function isObject(v) {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function getJSONKeys(jsonFile) {
  const content = fs.readFileSync(jsonFile, "utf-8");
  const v = JSON.parse(content);
  if (isObject(v)) {
    return Object.keys(v);
  }
  return [];
}

function verifyExports(names) {
  const hasDefaultExport = names.includes("default");
  const namedExports = Array.from(
    new Set(names.filter((name) => identRegexp.test(name) && !reservedWords.has(name))),
  );
  return {
    hasDefaultExport,
    namedExports,
  };
}

async function parseExports(input) {
  const { cwd, specifier, requireMode, nodeEnv = "production" } = input;
  const resolve = promisify(enhancedResolve.create({
    conditionNames: ["require", "node", "default"],
    extensions: [".cjs", ".js", ".json"],
    restrictions: [dirname(cwd)],
  }));
  const entry = specifier.startsWith("/") && /\.(js|cjs)$/.test(specifier)
    ? specifier
    : await resolve(cwd, specifier);
  const names = [];

  if (requireMode) {
    process.env.NODE_ENV = nodeEnv;
    const mod = require(entry);
    if (isObject(mod) || typeof mod === "function") {
      for (const key of Object.keys(mod)) {
        if (typeof key === "string" && key !== "") {
          names.push(key);
        }
      }
    }
    return verifyExports(names);
  }

  if (entry.endsWith(".json")) {
    return verifyExports(getJSONKeys(entry));
  }

  if (!entry.endsWith(".js") && !entry.endsWith(".cjs") && !entry.endsWith(".mjs")) {
    return verifyExports(names);
  }

  const requires = [{ path: entry, callMode: false }];
  while (requires.length > 0) {
    const req = requires.pop();
    try {
      const filename = req.path.replace(/\0/g, "");
      const code = fs.readFileSync(filename, "utf-8");
      const result = parse(filename, code, {
        nodeEnv,
        callMode: req.callMode,
      });
      if (
        result.reexports.length === 1
        && /^[a-z@]/i.test(result.reexports[0])
        && !result.reexports[0].endsWith("()")
        && !builtInNodeModules.has(result.reexports[0])
        && result.exports.length === 0
        && names.length === 0
      ) {
        return {
          reexport: result.reexports[0],
          hasDefaultExport: false,
          namedExports: [],
        };
      }
      names.push(...result.exports);
      for (let reexport of result.reexports) {
        const callMode = reexport.endsWith("()");
        if (callMode) {
          reexport = reexport.slice(0, -2);
        }
        if (builtInNodeModules.has(reexport)) {
          const mod = require(reexport);
          names.push(...Object.keys(mod));
        } else {
          const path = await resolve(dirname(filename), reexport);
          if (path.endsWith(".json")) {
            names.push(...getJSONKeys(path));
          } else {
            requires.push({ path, callMode });
          }
        }
      }
    } catch (err) {
      return Promise.reject(err);
    }
  }

  return verifyExports(names);
}

function readStdin() {
  return new Promise((resolve) => {
    let buf = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => (buf += chunk));
    process.stdin.on("end", () => resolve(buf));
  });
}

async function main() {
  try {
    const input = JSON.parse(await readStdin());
    const output = await parseExports(input);
    process.stdout.write(JSON.stringify(output));
  } catch (err) {
    process.stdout.write(JSON.stringify({ error: err.message, stack: err.stack }));
  }
  process.exit(0);
}

main();
