export type ImportMap = {
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
};

export type DenoConfig = ImportMap & {
  tasks?: Record<string, string>;
  importMap?: string;
};

export type Package = {
  readonly name: string;
  readonly version: string;
  readonly subModule?: string;
  readonly alias?: string;
  readonly dependencies?: Record<string, string>;
  readonly peerDependencies?: Record<string, string>;
  readonly exports?: Record<string, unknown> | string;
};

const importUrl = new URL(import.meta.url);
const VERSION = /^\/v\d+\/?/.test(importUrl.pathname)
  ? importUrl.pathname.split("/")[1]
  : "v{VERSION}";

// stable build for UI libraries like react, to make sure the runtime is single copy
const stableBuild = new Set([
  "preact",
  "react",
  "solid-js",
  "svelte",
  "vue",
  "@vue/reactivity",
  "@vue/runtime-core",
  "@vue/runtime-dom",
  "@vue/shared",
]);

let imFilename = "deno.json";

async function add(args: string[], options: Record<string, string>) {
  if (options.alias && args.length > 1) {
    console.error(
      `%cerror%c: Cannot use --alias with multiple packages`,
      "color:red",
      "",
    );
    Deno.exit(1);
  }

  const importMap = await loadImportMap();
  const pkgs = (await Promise.all(args.map(fetchPkgInfo))).filter(
    Boolean,
  ) as Package[];
  if (pkgs.length === 1 && options.alias) {
    (pkgs[0] as Record<string, unknown>).alias = options.alias;
  }

  await Promise.all(
    pkgs.map((pkg) => addPkgToImportMap(pkg, importMap)),
  );
  await saveImportMap(importMap);
  console.log(
    `Added ${pkgs.length} packages to %c${imFilename.split(/[\/\\]/g).pop()}`,
    "color:blue",
  );
  if (pkgs.length > 0) {
    console.log(
      pkgs.map((pkg, index) => {
        const tab = index === pkgs.length - 1 ? "└─" : "├─";
        const { name, version, subModule, alias } = pkg;
        let msg = `${tab} ${name}@${version}`;
        if (subModule) {
          msg = `${msg}/${subModule}`;
        }
        if (alias) {
          msg = `${msg} (alias: ${alias})`;
        }
        return msg;
      }).join("\n"),
    );
  }
}

async function update(args: string[], options: Record<string, string>) {
  const importMap = await loadImportMap();
  const imports = importMap.imports ?? {};
  const latest = "latest" in options;
  const toUpdate = args.length === 0
    ? Object.entries(imports).filter(([name, url]) =>
      url.startsWith(`${importUrl.origin}/`) &&
      !name.endsWith("/") &&
      !url.startsWith(`${importUrl.origin}/gh/`)
    ).map(([name, url]) => {
      let version: string;
      if (latest) {
        version = "latest";
      } else {
        const [, v] = url.match(/@(\d+\.\d+\.\d+(-[a-z0-9\-\.]+)?)/)!;
        if (!v.includes("-")) {
          version = v.split(".").slice(0, 2).join(".");
        } else {
          version = v;
        }
      }
      return `${name}@${version}`;
    })
    : args.filter((name) =>
      name in imports ||
      name.slice(0, name.lastIndexOf("@")) in imports
    ).map((name) => {
      let version: string;
      if ((name.startsWith("@") ? name.slice(1) : name).includes("@")) {
        const a = name.split("@");
        version = a.pop()!;
        name = a.join("@");
      } else if (latest) {
        version = "latest";
      } else {
        const url = imports[name] ??
          imports[name.slice(0, name.lastIndexOf("@"))];
        const [, v] = url.match(/@(\d+\.\d+\.\d+(-[a-z0-9\-\.]+)?)/)!;
        if (!v.includes("-")) {
          version = v.split(".").slice(0, 2).join(".");
        } else {
          version = v;
        }
      }
      return `${name}@${version}`;
    });
  const pkgs = (await Promise.all(toUpdate.map(fetchPkgInfo))).filter(
    Boolean,
  ) as Package[];
  const updates: Package[] = [];

  for (const pkg of pkgs) {
    if (await addPkgToImportMap(pkg, importMap)) {
      updates.push(pkg);
    }
  }

  await saveImportMap(importMap);
  console.log(`updates ${updates.length} packages`);
  if (updates.length > 0) {
    console.log(
      updates.map((pkg, index) => {
        const tab = index === pkgs.length - 1 ? "└─" : "├─";
        return `${tab} ${pkg.name}@${pkg.version}`;
      }).join("\n"),
    );
  }
}

async function remove(args: string[], _options: Record<string, string>) {
  const importMap = await loadImportMap();
  const { imports, scopes } = importMap;
  if (!imports) {
    return;
  }
  const toRemove = args.filter((name) => name in imports);
  for (const name of toRemove) {
    Reflect.deleteProperty(imports, name);
    Reflect.deleteProperty(imports, name + "/");
    if (scopes) {
      Reflect.deleteProperty(scopes, name);
      Reflect.deleteProperty(scopes, name + "/");
    }
  }
  await saveImportMap(importMap);
  console.log(`Removed ${toRemove.length} packages`);
  if (toRemove.length > 0) {
    console.log(
      toRemove.map((name, index) => {
        const tab = index === toRemove.length - 1 ? "└─" : "├─";
        return `${tab} ${name}`;
      }).join("\n"),
    );
  }
}

async function init(_args: string[], _options: Record<string, string>) {
  const config = await readDenoConfig();
  const tasks = config.tasks;
  config.tasks = {
    ...tasks,
    "esm:add": `deno run -A ${importUrl.origin}/${VERSION} add`,
    "esm:update": `deno run -A ${importUrl.origin}/${VERSION} update`,
    "esm:remove": `deno run -A ${importUrl.origin}/${VERSION} remove`,
  };
  await Deno.writeTextFile(
    "deno.json",
    await denoFmt(JSON.stringify(config, null, 4)),
  );
  console.log("Initialized %cdeno.json%c, 3 task added:", "color:green", "");
  console.log(
    "  - %cdeno task esm:add%c [packages...]",
    "color:blue",
    "color:gray",
  );
  console.log(
    "  - %cdeno task esm:update%c [packages...]",
    "color:blue",
    "color:gray",
  );
  console.log(
    "  - %cdeno task esm:remove%c [packages...]",
    "color:blue",
    "color:gray",
  );
}

const cache = new Map<string, Package>();
async function fetchPkgInfo(query: string): Promise<Package | null> {
  if (cache.has(query)) {
    return Promise.resolve(cache.get(query)!);
  }

  let pkgName: string;
  let alias: string | undefined;
  let subModule: string | undefined;
  if (query.includes(":")) {
    [alias, query] = query.split(":", 2);
  }
  if (!query) {
    throw new Error(`Invalid package name: "${query}"`);
  }
  const a = query.split("/").filter(Boolean);
  if (query.startsWith("@")) {
    if (a.length < 2) {
      return null;
    }
    pkgName = a[0] + "/" + a[1];
    if (a.length > 2) {
      subModule = a.slice(2).join("/");
    }
  } else {
    pkgName = a[0];
    if (a.length > 1) {
      subModule = a.slice(1).join("/");
    }
  }

  const res = await fetch(`${importUrl.origin}/${pkgName}/package.json`);
  if (res.status === 404) {
    console.error(`%cerror%c: Package "${pkgName}" not found`, "color:red", "");
    Deno.exit(1);
  }

  if (!res.ok) {
    console.error(`%cerror%c: Failed to fetch "${pkgName}"`, "color:red", "");
    console.error(await res.text());
    Deno.exit(1);
  }

  const pkg = await res.json();
  if (!pkg.name || !pkg.version) {
    console.error(
      `%cerror%c: Invalid package.json of "${pkgName}"`,
      "color:red",
      "",
    );
    Deno.exit(1);
  }

  pkg.alias = alias;
  pkg.subModule = subModule;
  cache.set(query, pkg);
  return pkg;
}

// https://github.com/denoland/fresh/blob/main/src/dev/mod.ts#L154-L170
async function denoFmt(code: string, ext = "json") {
  const proc = new Deno.Command(Deno.execPath(), {
    args: ["fmt", "--ext", ext, "-"],
    stdin: "piped",
    stdout: "piped",
    stderr: "null",
  }).spawn();

  const raw = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(code));
      controller.close();
    },
  });
  await raw.pipeTo(proc.stdin);
  const { stdout } = await proc.output();

  const formattedStr = new TextDecoder().decode(stdout);
  return formattedStr;
}

async function loadImportMap(): Promise<ImportMap> {
  try {
    const raw = await Deno.readTextFile(imFilename);
    return JSON.parse(raw);
  } catch (err) {
    if (err instanceof Deno.errors.NotFound) {
      return {};
    }
    throw err;
  }
}

async function saveImportMap(importMap: ImportMap): Promise<void> {
  const scopes = importMap.scopes ?? {};

  // clean up
  for (const importName in importMap.imports) {
    for (const [scopeName, scope] of Object.entries(scopes)) {
      if (importName in scope) {
        Reflect.deleteProperty(scope, importName);
        if (Object.keys(scope).length === 0) {
          Reflect.deleteProperty(scopes, scopeName);
        }
      }
    }
  }

  // sort
  const sortedImports = sortImports(importMap.imports);
  const sortedScopes = Object.fromEntries(
    Object.entries(scopes).sort(sortByKey).map((
      [key, scope],
    ) => [key, sortImports(scope)]),
  );

  // write
  await Deno.writeTextFile(
    imFilename,
    await denoFmt(
      JSON.stringify(
        { ...importMap, imports: sortedImports, scopes: sortedScopes },
        null,
        4,
      ),
    ),
  );
}

// todo: support deno.jsonc
let denoConfig: DenoConfig | undefined = undefined;
async function readDenoConfig(): Promise<DenoConfig> {
  if (denoConfig) {
    return denoConfig;
  }
  try {
    const config = await Deno.readTextFile("deno.json");
    denoConfig = JSON.parse(config);
    return denoConfig!;
  } catch (err) {
    if (err instanceof Deno.errors.NotFound) {
      return {};
    }
    throw err;
  }
}

async function addPkgToImportMap(
  pkg: Package,
  importMap: ImportMap,
): Promise<boolean> {
  let [pkgUrl, withExports] = getPkgUrl(pkg);
  let aliasName = pkg.alias ?? pkg.name;
  if (pkg.subModule) {
    if (!pkg.alias) {
      aliasName += "/" + pkg.subModule;
    }
    pkgUrl += "/" + pkg.subModule;
  }
  if (!importMap.imports) {
    importMap.imports = {};
  }
  if (importMap.imports[aliasName] === pkgUrl) {
    return false;
  }
  importMap.imports[aliasName] = pkgUrl;
  if (withExports && !pkg.subModule) {
    importMap.imports[aliasName + "/"] = pkgUrl + "/";
  } else {
    Reflect.deleteProperty(importMap.imports, aliasName + "/");
  }
  if (pkg.dependencies) {
    const esmshScope = `${importUrl.origin}/${VERSION}/`;
    if (!importMap.scopes) {
      importMap.scopes = {};
    }
    if (!Reflect.has(importMap.scopes, esmshScope)) {
      importMap.scopes[esmshScope] = {};
    }
    for (const [depName, depVersion] of Object.entries(pkg.dependencies)) {
      const dep = `${depName}@${depVersion}`;
      const depPkg = await fetchPkgInfo(dep);
      if (depPkg) {
        const depUrl =
          `${importUrl.origin}/${VERSION}/${depPkg.name}@${depPkg.version}`;
        importMap.scopes[esmshScope][depName] = depUrl;
      }
    }
  }
  return true;
}

function getPkgUrl(pkg: Package): [url: string, withExports: boolean] {
  const { name, version, exports, dependencies, peerDependencies } = pkg;
  const withExports = typeof exports === "object" &&
    Object.keys(exports).some((key) =>
      key.startsWith("./") && key !== "./package.json"
    );
  if (
    !stableBuild.has(name) && (
      (dependencies && Object.keys(dependencies).length > 0) ||
      (peerDependencies && Object.keys(peerDependencies).length > 0)
    )
  ) {
    return [`${importUrl.origin}/${VERSION}/*${name}@${version}`, withExports];
  }
  return [`${importUrl.origin}/${VERSION}/${name}@${version}`, withExports];
}

function sortImports(imports?: Record<string, string>): typeof imports {
  if (!imports) {
    return undefined;
  }
  return Object.fromEntries(
    Object.entries(imports).sort(sortByValue),
  );
}

function sortByKey(a: [string, unknown], b: [string, unknown]) {
  const [aName] = a;
  const [bName] = b;
  if (aName < bName) {
    return -1;
  }
  if (aName > bName) {
    return 1;
  }
  return 0;
}

function sortByValue(a: [string, string], b: [string, string]) {
  const aValue = a[1].replace("/*", "/");
  const bValue = b[1].replace("/*", "/");
  if (aValue < bValue) {
    return -1;
  }
  if (aValue > bValue) {
    return 1;
  }
  return 0;
}

function parseFlags(
  raw: string[],
): [args: string[], options: Record<string, string>] {
  const args: string[] = [];
  const options: Record<string, string> = {};
  let argCur: string | null = null;
  for (const arg of raw) {
    if (arg.startsWith("--")) {
      if (argCur) {
        options[argCur] = "";
        argCur = null;
      }
      if (arg.includes("=")) {
        const [name, value] = arg.slice(2).split("=");
        options[name] = value;
      } else {
        argCur = arg.slice(2);
      }
    } else if (argCur) {
      options[argCur] = arg;
      argCur = null;
    } else {
      args.push(arg);
    }
  }
  if (argCur) {
    options[argCur] = "";
    argCur = null;
  }
  return [args, options];
}

function isNEString(a: unknown): a is string {
  return typeof a === "string" && a !== "";
}

if (import.meta.main) {
  const start = performance.now();
  const [command, ...args] = Deno.args;
  const commands = { add, remove, update, init };

  if (command === undefined || !(command in commands)) {
    console.error(`%cerror%c: Invalid command "${command}"`, "color:red", "");
    Deno.exit(1);
  }

  try {
    const flags = parseFlags(args);
    const config = await readDenoConfig();
    if (isNEString(config.importMap)) {
      imFilename = config.importMap;
    }
    await commands[command as keyof typeof commands](...flags);
    console.log(`✨ Done in ${(performance.now() - start).toFixed(2)}ms`);
  } catch (error) {
    throw error;
  }
}
