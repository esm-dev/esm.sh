const { readFileSync, existsSync, writeFileSync } = require("fs");
const { dirname, join } = require("path");
const { parse } = require("esm-cjs-lexer");
const { env, stdin, stdout } = process;

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
  "assert/strict",
  "async_hooks",
  "child_process",
  "cluster",
  "buffer",
  "console",
  "constants",
  "crypto",
  "dgram",
  "diagnostics_channel",
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
  "string_decoder",
  "sys",
  "timers",
  "timers/promises",
  "tls",
  "trace_events",
  "tty",
  "url",
  "util",
  "util/types",
  "v8",
  "vm",
  "wasi",
  "webcrypto",
  "worker_threads",
  "zlib",
]);

function isNEString(v) {
  return typeof v === "string" && v.length > 0;
}

function isObject(v) {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function resolveExport(v) {
  if (Array.isArray(v)) {
    for (const e of v) {
      if (isObject(e)) {
        if (e.require) {
          return e.require;
        }
        if (e.node) {
          return e.node;
        }
      } else if (isNEString(e)) {
        return e;
      }
    }
  } else if (isObject(v)) {
    const cjs = v.require ?? v.node ?? v.default;
    if (isNEString(cjs)) {
      return cjs;
    }
    if (isObject(cjs) && isNEString(cjs.default)) {
      return cjs.default;
    }
  } else if (isNEString(v)) {
    return v;
  }
  return null;
}

function getJSONKeys(jsonFile) {
  const content = readFileSync(jsonFile, "utf-8");
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
  const [buildPkgName, wd, specifier, nodeEnv, requireMode] = input;
  const exportNames = [];
  const entry = specifier.startsWith("/") ? specifier : resolve(specifier);

  function resolve(specifier, containingFilename) {
    if (specifier.startsWith("file://") || specifier.startsWith("/")) {
      return specifier;
    }
    if (specifier === "." && containingFilename) {
      const dir = dirname(containingFilename);
      let path = join(dir, "index.js");
      if (existsSync(path)) {
        return path;
      }
      path = join(dir, "index.cjs");
      if (existsSync(path)) {
        return path;
      }
    }
    if (specifier === ".." && containingFilename) {
      const dir = dirname(containingFilename);
      let path = join(dir, "../index.js");
      if (existsSync(path)) {
        return path;
      }
      path = join(dir, "../index.cjs");
      if (existsSync(path)) {
        return path;
      }
    }
    if (specifier.startsWith("./") || specifier.startsWith("../")) {
      if (containingFilename) {
        return join(dirname(containingFilename), specifier);
      }
      return join(wd, "node_modules", buildPkgName, specifier);
    }
    const segments = specifier.split("/");
    let pkgName = segments[0];
    let subModule = segments.slice(1).join("/");
    if (specifier.startsWith("@") && segments.length > 1) {
      pkgName = segments[0] + "/" + segments[1];
      subModule = segments.slice(2).join("/");
    }
    let pkgDir = join(wd, "node_modules", pkgName);
    let pkgJson = join(pkgDir, "package.json");
    if (!existsSync(pkgJson)) {
      if (wd.includes("/node_modules/.pnpm/")) {
        pkgDir = join(wd.split("/node_modules/.pnpm/")[0], "node_modules", ".pnpm", "node_modules", pkgName);
        pkgJson = join(pkgDir, "package.json");
      } else {
        pkgDir = join(wd, "node_modules", ".pnpm", "node_modules", pkgName);
        pkgJson = join(pkgDir, "package.json");
      }
    }
    if (existsSync(pkgJson)) {
      if (subModule && existsSync(join(pkgDir, subModule, "package.json"))) {
        pkgDir = join(pkgDir, subModule);
        pkgJson = join(pkgDir, "package.json");
        const pkg = JSON.parse(readFileSync(pkgJson, "utf-8"));
        return join(pkgDir, pkg.main ?? "index.js");
      }
      const pkg = JSON.parse(readFileSync(pkgJson, "utf-8"));
      if (subModule === "") {
        if (isObject(pkg.exports) && pkg.exports["."]) {
          const path = resolveExport(pkg.exports["."]);
          if (path) {
            return join(pkgDir, path);
          }
        } else {
          return join(pkgDir, pkg.main ?? "index.js");
        }
      } else {
        if (isObject(pkg.exports)) {
          if (pkg.exports["./" + subModule]) {
            const path = resolveExport(pkg.exports["./" + subModule]);
            if (path) {
              return join(pkgDir, path);
            }
          }
          // exports: "./*": "./dist/*.js"
          for (const key of Object.keys(pkg.exports)) {
            if (key.startsWith("./") && key.endsWith("/*")) {
              if (("./" + subModule).startsWith(key.slice(0, -1))) {
                const path = resolveExport(pkg.exports[key]);
                if (path) {
                  return join(pkgDir, path.replace("*", subModule.slice(2, -1)));
                }
              }
            }
          }
        }
        return join(pkgDir, subModule);
      }
    }

    throw new Error(
      `Cannot resolve module '${specifier}'` + (containingFilename ? ` from '${containingFilename}' ` : "") + " in " + wd,
    );
  }

  if (requireMode) {
    env.NODE_ENV = nodeEnv;
    const mod = require(entry);
    if (isObject(mod) || typeof mod === "function") {
      for (const key of Object.keys(mod)) {
        if (typeof key === "string") {
          exportNames.push(key);
        }
      }
    }
    return verifyExports(exportNames);
  }

  if (entry.endsWith(".json")) {
    return verifyExports(getJSONKeys(entry));
  }

  const requires = [{ path: entry, callMode: false }];
  while (requires.length > 0) {
    const req = requires.pop();
    let filename = req.path.replace(/\0/g, "");
    if (!filename.endsWith(".js") && !filename.endsWith(".cjs")) {
      if (existsSync(filename + ".js")) {
        filename += ".js";
      } else if (existsSync(filename + ".cjs")) {
        filename += ".cjs";
      } else if (existsSync(join(filename, "index.js"))) {
        filename = join(filename, "index.js");
      } else if (existsSync(join(filename, "index.cjs"))) {
        filename = join(entry, "index.cjs");
      }
    }
    const result = parse(
      filename,
      readFileSync(filename, "utf-8"),
      {
        nodeEnv,
        callMode: req.callMode,
      },
    );
    if (
      result.reexports.length === 1
      && /^[a-z@]/i.test(result.reexports[0])
      && !result.reexports[0].endsWith("()")
      && !builtInNodeModules.has(result.reexports[0])
      && result.exports.length === 0
      && exportNames.length === 0
    ) {
      return {
        reexport: result.reexports[0],
        hasDefaultExport: false,
        namedExports: [],
      };
    }
    exportNames.push(...result.exports);
    for (let reexport of result.reexports) {
      const callMode = reexport.endsWith("()");
      if (callMode) {
        reexport = reexport.slice(0, -2);
      }
      if (builtInNodeModules.has(reexport)) {
        const mod = require(reexport);
        exportNames.push(...Object.keys(mod));
      } else {
        const path = resolve(reexport, filename);
        if (path.endsWith(".json")) {
          exportNames.push(...getJSONKeys(path));
        } else {
          requires.push({ path, callMode });
        }
      }
    }
  }
  return verifyExports(exportNames);
}

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

async function main() {
  try {
    const input = JSON.parse(await readStdin());
    const output = await parseExports(input);
    stdout.write(JSON.stringify(output));
  } catch (err) {
    stdout.write(JSON.stringify({ error: err.message, stack: err.stack }));
  }
  process.exit(0);
}

main();
