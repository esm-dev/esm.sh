export type ImportMap = {
  readonly imports: Record<string, string>;
  readonly scopes: Record<string, Record<string, string>>;
};

export type Package = {
  readonly name: string;
  readonly version: string;
  readonly dependencies?: Record<string, string>;
  readonly peerDependencies?: Record<string, string>;
  readonly exports?: Record<string, unknown> | string;
};

const VERSION = "v{VERSION}";

async function add(args: string[]) {
  const pkgs = await Promise.all(args.map(fetchPkgInfo));
  const importMap: ImportMap = { imports: {}, scopes: {} };
  try {
    const { imports, scopes } = JSON.parse(
      await Deno.readTextFile("import_map.json"),
    );
    if (imports) {
      Object.assign(importMap.imports, imports);
    }
    if (scopes) {
      Object.assign(importMap.scopes, scopes);
    }
  } catch (err) {
    if (!(err instanceof Deno.errors.NotFound)) {
      throw err;
    }
  }

  for (const pkg of pkgs) {
    const { name, version, dependencies } = pkg;
    const url = new URL(`https://esm.sh/${name}@${version}`);
    url.searchParams.set("external", "*");
    url.searchParams.set("pin", VERSION);
    url.searchParams.sort();
    importMap.imports[name] = url.href;
    if (
      typeof pkg.exports === "object" &&
      Object.keys(pkg.exports).some((key) =>
        key.length >= 3 && key.startsWith("./")
      )
    ) {
      importMap.imports[name + "/"] = `${url.href}&path=/`;
    }
    if (pkg) {
      if (dependencies) {
        importMap.scopes[name] = {};
        for (const [depName, depVersion] of Object.entries(dependencies)) {
          const dep = `${depName}@${depVersion}`;
          const depPkg = await fetchPkgInfo(dep);
          const depUrl = new URL(
            `https://esm.sh/${depPkg.name}@${depPkg.version}`,
          );
          depUrl.searchParams.set("pin", VERSION);
          importMap.scopes[name][depName] = depUrl.href;
        }
      }
    }
  }

  // clean up
  for (const importName in importMap.imports) {
    for (const [scopeName, scope] of Object.entries(importMap.scopes)) {
      if (importName in scope) {
        Reflect.deleteProperty(scope, importName);
        if (Object.keys(scope).length === 0) {
          Reflect.deleteProperty(importMap.scopes, scopeName);
        }
      }
    }
  }

  await Deno.writeTextFile(
    "import_map.json",
    JSON.stringify(importMap, null, 2),
  );
  console.log(`Added ${pkgs.length} packages to import map.`);
  console.log(
    pkgs.map((pkg, index) => {
      const tab = index === pkgs.length - 1 ? "└─" : "├─";
      return `${tab} ${pkg.name}@${pkg.version}`;
    }).join("\n"),
  );
}

async function remove(args: string[]) {
  console.log("todos: remove command");
}

async function upgrade(args: string[]) {
  console.log("todos: upgrade command");
}

const cache = new Map<string, Package>();
async function fetchPkgInfo(name: string): Promise<Package> {
  if (cache.has(name)) {
    return Promise.resolve(cache.get(name)!);
  }

  const res = await fetch(`https://esm.sh/${name}/package.json`);
  if (!res.ok) {
    throw new Error(`Failed to fetch "${name}"`);
  }

  const pkg = await res.json();
  if (!pkg.name || !pkg.version) {
    throw new Error(`Invalid package.json of "${name}"`);
  }

  cache.set(name, pkg);
  return pkg;
}

if (import.meta.main) {
  const [command, ...args] = Deno.args;
  const commands = {
    add,
    remove,
    upgrade,
  };

  if (command === undefined || !(command in commands)) {
    console.error(`Command "${command}" not found`);
    Deno.exit(1);
  }

  await commands[command as keyof typeof commands](args);
}
