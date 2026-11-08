import { assert, assertEquals } from "jsr:@std/assert";

for (const bundle of ["", "&bundle", "&bundle=false"]) {
  const query = "?wasm=instance&target=es2022" + bundle;

  Deno.test("tiktoken" + bundle, async () => {
    const { get_encoding } = await import("esm.sh/tiktoken@1.0.22" + query);
    const encoding = get_encoding("cl100k_base");
    try {
      const text = "Hello, WebAssembly!";
      const tokens = encoding.encode(text);
      assert(tokens.length > 0);
      assertEquals(new TextDecoder().decode(encoding.decode(tokens)), text);
    } finally {
      encoding.free();
    }
  });

  Deno.test("rapier" + bundle, async () => {
    const { World, RigidBodyDesc, ColliderDesc } = await import("esm.sh/@dimforge/rapier3d@0.21.0" + query);
    const world = new World({ x: 0, y: -9.81, z: 0 });
    try {
      const body = world.createRigidBody(RigidBodyDesc.dynamic().setTranslation(0, 10, 0));
      world.createCollider(ColliderDesc.ball(1), body);
      for (let i = 0; i < 60; i++) world.step();
      assert(body.translation().y < 6);
    } finally {
      world.free();
    }
  });

  Deno.test("automerge" + bundle, async () => {
    const { from, clone, change, merge, save, load } = await import("esm.sh/@automerge/automerge@3.5.0" + query);
    let a = from({ items: [] });
    let b = clone(a);
    a = change(a, (doc: { items: string[] }) => doc.items.push("a"));
    b = change(b, (doc: { items: string[] }) => doc.items.push("b"));
    assertEquals([...load(save(merge(a, b))).items].sort(), ["a", "b"]);
  });
}
