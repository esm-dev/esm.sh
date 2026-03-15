import { assertStringIncludes } from "jsr:@std/assert";

// Test: scoped fork packages should resolve bare imports as self-references.
// When @needle-tools/three (a scoped fork of three.js) internally imports "three",
// it should resolve to itself, not to upstream "three@latest".

Deno.test("scoped fork: bare import resolves to self-reference", async () => {
  // Fetch a sub-module that explicitly imports from bare "three" internally.
  // Gyroscope.js starts with: import { Object3D, Quaternion } from 'three';
  // Without the fix, esm.sh would resolve this to the upstream "three" package,
  // producing an import like `from"/three@..."` instead of `from"/@needle-tools/three@..."`.
  const res = await fetch(
    "http://localhost:8080/@needle-tools/three@0.169.19/examples/jsm/misc/Gyroscope.js?target=es2022",
  );
  const text = await res.text();
  // The resolved import must reference @needle-tools/three (self-ref), NOT bare "/three@..."
  assertStringIncludes(text, "/@needle-tools/three@");
});

Deno.test("scoped fork: bare import does NOT resolve to upstream package", async () => {
  // Negative test: if the fix regresses, esm.sh would resolve `three` to the upstream
  // package, producing a path like `from"/three@0.xxx/..."` in the output.
  const res = await fetch(
    "http://localhost:8080/@needle-tools/three@0.169.19/examples/jsm/misc/Gyroscope.js?target=es2022",
  );
  const text = await res.text();
  // Must NOT contain a reference to the upstream /three@ package
  const badImports = text.split("\n").filter(
    (l) => l.includes('"/three@') && !l.includes("@needle-tools/three"),
  );
  if (badImports.length > 0) {
    throw new Error(
      `Bare 'three' was resolved to upstream package instead of self-reference:\n${badImports.join("\n")}`,
    );
  }
});
