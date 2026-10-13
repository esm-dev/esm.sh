import { assertStrictEquals } from "jsr:@std/assert";
import {
  webgl2Adapter,
  WebGLDevice,
} from "http://localhost:8080/@luma.gl/webgl@9.3.3?deps=@luma.gl/core@9.3.3&target=es2022";

Deno.test("static and dynamic imports share the WebGLDevice class", async () => {
  const device = Object.create(WebGLDevice.prototype) as WebGLDevice;
  assertStrictEquals(await webgl2Adapter.attach(device), device);
});
