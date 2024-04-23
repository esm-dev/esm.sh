import { assert } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { onMount } from "http://localhost:8080/svelte@4.2.15?target=es2022";
import { onMount as onMount_ } from "http://localhost:8080/svelte@4.2.15/internal?target=es2022";

// using ssr.js for deno
import { onMount as onMountSSR } from "http://localhost:8080/svelte@4.2.15";
import { onMount as onMountSSR_ } from "http://localhost:8080/svelte@4.2.15/internal";

Deno.test("svelte runtime", async () => {
  assert(onMount === onMount_);
});

Deno.test("svelte runtime (SSR using _noop_ `onMount`)", async () => {
  assert(onMountSSR !== onMountSSR_);
});
