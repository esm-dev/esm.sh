# esm-bundle

Bundling multiple files into a single binary blob.

```js
import { Bundle } from "esm-bundle";

// bundle some files
const data = Bundle.bundle([
  { name: "foo.txt", type: "text/plain", lastModified: 0, content: new Uint8Array([1, 2, 3]) },
  { name: "bar.txt", type: "text/plain", lastModified: 0, content: new Uint8Array([4, 5, 6]) },
]);

// use the bundle
const bundle = new Bundle(data);
bundle.checksum; // a 32-bit checksum of the bundle
bundle.entries[0].name; // => "foo.txt"
bundle.entries[0].type; // => "text/plain"
bundle.entries[1].name; // => "bar.txt"
bundle.entries[1].type; // => "text/plain"
bundle.readFile("foo.txt"); // => File([1, 2, 3], "foo.txt", { type: "text/plain" })
bundle.readFile("bar.txt"); // => File([4, 5, 6], "bar.txt", { type: "text/plain" })
```

## Gzip

This library does not compress the bundle files. Here is an example of how to use `CompressionStream` to compress data
using gzip, and `DecompressionStream` to decompress it again.

```js
// bundle some files
const data = Bundle.bundle([/* ... */]);

// compress the bundle using CompressionStream
const compressed = await new Response(data).body.pipeThrough(new CompressionStream("gzip")).arrayBuffer();

// use the compressed bundle
const decompressed = await new Response(compressed).body.pipeThrough(new DecompressionStream("gzip")).arrayBuffer();
const bundle = new Bundle(decompressed);
```

> Note that `CompressionStream` and `DecompressionStream` are not supported in all browsers, and you may need to use a
> polyfill or a different compression algorithm depending on your requirements.
