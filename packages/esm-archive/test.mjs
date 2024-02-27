import { writeFileSync } from "fs";
import { Archive, bundle } from "./index.mjs";

const lastModified = 1708915929964;
const lastModified2 = Math.round(lastModified / 1000) * 1000;
const randomString = () =>
  Array.from({ length: Math.round(Math.random() * 1000) }, () => Math.random().toString(36)[2]).join("");

const file0 = new File([randomString()], "foo.txt", { type: "text/foo", lastModified });
const file1 = new File([randomString()], "bar.txt", { type: "text/bar", lastModified });
const data = await bundle([file0, file1]);
const archive = new Archive(data);

if (archive.entries.length !== 2) throw new Error("invalid entries");
if (archive.entries[0].name !== "foo.txt") throw new Error("invalid entries");
if (archive.entries[0].type !== "text/foo") throw new Error("invalid entries");
if (archive.entries[0].lastModified !== lastModified2) throw new Error("invalid entries");
if (archive.entries[0].size !== file0.size) throw new Error("invalid entries");
if (archive.entries[1].name !== "bar.txt") throw new Error("invalid entries");
if (archive.entries[1].type !== "text/bar") throw new Error("invalid entries");
if (archive.entries[1].lastModified !== lastModified2) throw new Error("invalid entries");
if (archive.entries[1].size !== file1.size) throw new Error("invalid entries");
if (await archive.openFile("foo.txt").text() !== await file0.text()) throw new Error("invalid foo.txt");
if (await archive.openFile("bar.txt").text() !== await file1.text()) throw new Error("invalid bar.txt");

console.log("ok");
console.log("chekcsum", archive.checksum);

writeFileSync(
  "../../server/embed/esm-archive.bin",
  await bundle([
    new File(['export const foo = "bar";'], "http://localhost:8080/foo.js", { type: "application/javascript", lastModified }),
  ]),
);
