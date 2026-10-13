import { assertEquals } from "jsr:@std/assert";

import { blue as blue7, generate as generate7 } from "http://localhost:8080/@ant-design/colors@7.2.1?target=es2022";
import { blue as blue7Dev, generate as generate7Dev } from "http://localhost:8080/@ant-design/colors@7.2.1?target=es2022&dev";
import { blue as blue8, generate as generate8 } from "http://localhost:8080/@ant-design/colors@8.0.0?target=es2022";
import { blue as blue8Dev, generate as generate8Dev } from "http://localhost:8080/@ant-design/colors@8.0.0?target=es2022&dev";

Deno.test("Ant Design color exports", () => {
  for (const [blue, generate] of [[blue7, generate7], [blue7Dev, generate7Dev], [blue8, generate8], [blue8Dev, generate8Dev]] as const) {
    assertEquals(blue.length, 10);
    assertEquals(blue.primary, "#1677ff");
    assertEquals(generate(blue.primary!), [...blue]);
  }
});
