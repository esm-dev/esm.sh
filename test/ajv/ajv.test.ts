import { assert } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import Ajv from "http://localhost:8080/ajv";
import addFormats from "http://localhost:8080/ajv-formats";

Deno.test("ajv", () => {
  const ajv = new Ajv({ strictTypes: false });
  addFormats(ajv, ["date", "time"]);

  const validateDate = ajv.compile({ format: "date" });
  assert(validateDate("2020-09-17"));
  assert(!validateDate("2020-09-35"));
});
