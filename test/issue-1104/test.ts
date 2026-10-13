import { assertInstanceOf } from "jsr:@std/assert";

for (const query of ["", "&dev"]) {
  Deno.test({
    name: `issue #1104 - UTApi server detection in Deno 2${query}`,
    ignore: Deno.version.deno.startsWith("1."),
    async fn() {
      const { UTApi } = await import(
        `esm.sh/uploadthing@7.5.2/server?target=denonext&bundle${query}`
      );
      assertInstanceOf(new UTApi(), UTApi);
    },
  });
}
