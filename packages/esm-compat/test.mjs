import { getBuildTargetFromUA } from "./dist/compat.js";

const testData = {
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36":
    "es2022",
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15":
    "es2021",
    "Deno/1.33.1": "deno",
    "Deno/1.33.2": "denonext",
};

for (const [ua, expected] of Object.entries(testData)) {
  const actual = getBuildTargetFromUA(ua);
  if (actual !== expected) {
    console.error(`UA ${ua} should have been ${expected} but was ${actual}`);
    process.exit(1);
  }
}

console.log("✅ All tests passed");
