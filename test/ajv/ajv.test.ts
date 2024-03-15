import { assert } from "https://deno.land/std@0.220.0/assert/mod.ts";

import Ajv from "http://localhost:8080/ajv@8.12.0";
import addFormats from "http://localhost:8080/ajv-formats@2.1.1?deps=ajv@8.12.0";

Deno.test("ajv", () => {
  const ajv = new Ajv({ strictTypes: false });
  addFormats(ajv, ["date", "time"]);

  const validateDate = ajv.compile({ format: "date" });
  assert(validateDate("2020-09-17"));
  assert(!validateDate("2020-09-35"));
});
