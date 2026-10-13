import { assertEquals } from "jsr:@std/assert";
import process, {
  version,
  versions,
} from "http://localhost:8080/node/process.mjs";

// Browser builds must not see Deno's Node-compatible process.
Reflect.set(globalThis, "process", process);
Reflect.set(globalThis, "window", globalThis);

Deno.test("browser process does not identify as Node.js", () => {
  assertEquals(process.version, "");
  assertEquals(version, "");
  assertEquals<unknown>(process.versions, {});
  assertEquals<unknown>(versions, {});
});

for (const query of ["target=es2022", "target=es2022&standalone"]) {
  Deno.test(`issue #1134 - lerc browser WASM (${query})`, async () => {
    const lerc = await import(`http://localhost:8080/lerc@4.2.0?${query}`);
    await lerc.load({
      locateFile: () => "http://localhost:8080/lerc@4.2.0/lerc-wasm.wasm",
    });
    assertEquals(lerc.isLoaded(), true);
  });
}
