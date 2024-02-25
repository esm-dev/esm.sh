import { Bundle } from "./index.mjs";

const now = Date.now();
const nowUnix = Math.round(now / 1000);
const encoder = new TextEncoder();
const randomString = (n) => Array.from({ length: n }, () => Math.random().toString(36)[2]).join("");
const randomStrings = [
  randomString(1000 + Math.round(Math.random() * 1000)),
  randomString(1000 + Math.round(Math.random() * 1000)),
];

const buffer = await Bundle.bundle([
  { name: "foo.txt", type: "text/foo", lastModified: now, content: encoder.encode(randomStrings[0]) },
  { name: "bar.txt", type: "text/bar", lastModified: now, content: encoder.encode(randomStrings[1]) },
]);

const bundle = new Bundle(buffer);

if (bundle.entries.length !== 2) throw new Error("invalid entries");
if (bundle.entries[0].name !== "foo.txt") throw new Error("invalid entries");
if (bundle.entries[0].type !== "text/foo") throw new Error("invalid entries");
if (bundle.entries[0].lastModified !== nowUnix * 1000) throw new Error("invalid entries");
if (bundle.entries[0].length !== encoder.encode(randomStrings[0]).length) throw new Error("invalid entries");
if (bundle.entries[1].name !== "bar.txt") throw new Error("invalid entries");
if (bundle.entries[1].type !== "text/bar") throw new Error("invalid entries");
if (bundle.entries[1].lastModified !== nowUnix * 1000) throw new Error("invalid entries");
if (bundle.entries[1].length !== encoder.encode(randomStrings[1]).length) throw new Error("invalid entries");
if (await bundle.readFile("foo.txt").text() !== randomStrings[0]) throw new Error("invalid foo.txt");
if (await bundle.readFile("bar.txt").text() !== randomStrings[1]) throw new Error("invalid bar.txt");

console.log("ok");
console.log("chekcsum", bundle.checksum);
