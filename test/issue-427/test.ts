import { assert, assertEquals } from "jsr:@std/assert";

for (
  const suffix of [
    "",
    "?target=es2022",
    "?bundle",
    "?dev",
    "?target=es2022&bundle",
    "/benchmark.js?target=es2022",
  ]
) {
  Deno.test(`benchmark Suite ${suffix}`, async () => {
    const { default: Benchmark } = await import(
      `http://localhost:8080/benchmark@2.1.4${suffix}`
    );
    const suite = new Benchmark.Suite("example");
    let calls = 0;
    let completed = false;
    suite.add("increment", () => calls++, {
      minSamples: 1,
      minTime: 0.001,
      maxTime: 0.001,
    });
    suite.on("complete", () => completed = true);
    suite.run();
    assert(completed);
    assert(calls > 0);
    assertEquals(suite[0].error, undefined);
    assertEquals(suite.filter("successful").map("name"), ["increment"]);
    assertEquals(typeof Benchmark.platform.parse, "function");
    assertEquals(
      new (Benchmark.runInContext().Suite)("context").name,
      "context",
    );
  });
}
