import { Archive } from "./index.mjs";

const now = Date.now();
const nowUnix = Math.round(now / 1000);
const randomString = () =>
  Array.from({ length: Math.round(Math.random() * 1000) }, () => Math.random().toString(36)[2]).join("");

const file0 = new File([randomString()], "foo.txt", { type: "text/foo", lastModified: now });
const file1 = new File([randomString()], "bar.txt", { type: "text/bar", lastModified: now });
const bundle = await Archive.bundle([file0, file1]);
const archive = new Archive(bundle);

if (archive.entries.length !== 2) throw new Error("invalid entries");
if (archive.entries[0].name !== "foo.txt") throw new Error("invalid entries");
if (archive.entries[0].type !== "text/foo") throw new Error("invalid entries");
if (archive.entries[0].lastModified !== nowUnix * 1000) throw new Error("invalid entries");
if (archive.entries[0].size !== file0.size) throw new Error("invalid entries");
if (archive.entries[1].name !== "bar.txt") throw new Error("invalid entries");
if (archive.entries[1].type !== "text/bar") throw new Error("invalid entries");
if (archive.entries[1].lastModified !== nowUnix * 1000) throw new Error("invalid entries");
if (archive.entries[1].size !== file1.size) throw new Error("invalid entries");
if (await archive.readFile("foo.txt").text() !== await file0.text()) throw new Error("invalid foo.txt");
if (await archive.readFile("bar.txt").text() !== await file1.text()) throw new Error("invalid bar.txt");

console.log("ok");
console.log("chekcsum", archive.checksum);
