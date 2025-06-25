/**
 * Test the "at" query parameter functionality for date-based version resolution
 */

import { assertEquals } from "jsr:@std/assert";

const origin = Deno.env.get("ORIGIN") || "http://localhost:8080";

Deno.test("at parameter with date formats", async () => {
  // Test date-based version resolution
  const tests = [
    {
      name: "Year only format",
      url: `${origin}/lodash?at=2023`,
      description: "Should resolve to latest lodash version as of 2023-01-01",
    },
    {
      name: "Year-month format", 
      url: `${origin}/lodash?at=2024-05`,
      description: "Should resolve to latest lodash version as of 2024-05-01",
    },
    {
      name: "Full date format",
      url: `${origin}/lodash?at=2025-01-15`, 
      description: "Should resolve to latest lodash version as of 2025-01-15",
    },
  ];

  for (const test of tests) {
    const response = await fetch(test.url);
    
    // Should redirect to an exact version
    assertEquals(response.status, 302, `${test.name}: Expected redirect`);
    
    const location = response.headers.get("location");
    if (location) {
      // Location should contain an exact version (e.g., lodash@4.17.21/es2022/lodash.mjs)
      const versionMatch = location.match(/lodash@(\d+\.\d+\.\d+)/);
      assertEquals(
        versionMatch !== null,
        true,
        `${test.name}: Should redirect to exact version, got: ${location}`
      );
      
      console.log(`✓ ${test.name}: ${test.url} -> ${versionMatch![1]}`);
    }
  }
});

Deno.test("at parameter with unix timestamp", async () => {
  // Test unix timestamp format
  const timestamp = "1672531200"; // 2023-01-01 00:00:00 UTC
  const url = `${origin}/lodash?at=${timestamp}`;
  
  const response = await fetch(url);
  assertEquals(response.status, 302, "Expected redirect for timestamp");
  
  const location = response.headers.get("location");
  if (location) {
    const versionMatch = location.match(/lodash@(\d+\.\d+\.\d+)/);
    assertEquals(
      versionMatch !== null,
      true,
      `Should redirect to exact version, got: ${location}`
    );
    
    console.log(`✓ Unix timestamp: ${url} -> ${versionMatch![1]}`);
  }
});

Deno.test("at parameter with version constraints", async () => {
  // Test version constraints with date
  const tests = [
    {
      name: "Major version constraint with date",
      url: `${origin}/lodash@4?at=2024-01-01`,
      expectedPattern: /lodash@4\.\d+\.\d+/,
    },
    {
      name: "Semver range with date", 
      url: `${origin}/lodash@^4.0.0?at=2023-06-01`,
      expectedPattern: /lodash@4\.\d+\.\d+/,
    },
  ];

  for (const test of tests) {
    const response = await fetch(test.url);
    assertEquals(response.status, 302, `${test.name}: Expected redirect`);
    
    const location = response.headers.get("location");
    if (location) {
      assertEquals(
        test.expectedPattern.test(location),
        true,
        `${test.name}: Should match pattern ${test.expectedPattern}, got: ${location}`
      );
      
      const versionMatch = location.match(/lodash@(\d+\.\d+\.\d+)/);
      console.log(`✓ ${test.name}: ${test.url} -> ${versionMatch![1]}`);
    }
  }
});

Deno.test("at parameter should be ignored for exact versions", async () => {
  // When exact version is specified, at parameter should be ignored
  const url = `${origin}/lodash@4.17.21?at=2020-01-01`;
  
  const response = await fetch(url);
  assertEquals(response.status, 302, "Expected redirect for exact version");
  
  const location = response.headers.get("location");
  if (location) {
    // Should still resolve to the exact version specified, ignoring the date
    assertEquals(
      location.includes("lodash@4.17.21"),
      true,
      `Exact version should be preserved, got: ${location}`
    );
    
    console.log(`✓ Exact version preserved: ${url} -> lodash@4.17.21`);
  }
});

Deno.test("invalid at parameter formats", async () => {
  const invalidFormats = [
    "invalid-date",
    "2024-13-01", // Invalid month
    "2024-01-32", // Invalid day
    "not-a-number",
    "2024-02-30", // Invalid date
  ];

  for (const format of invalidFormats) {
    const url = `${origin}/lodash?at=${format}`;
    const response = await fetch(url);
    
    assertEquals(
      response.status,
      400,
      `Invalid format "${format}" should return 400 status`
    );
    
    const text = await response.text();
    assertEquals(
      text.includes("Invalid at parameter"),
      true,
      `Error message should mention invalid at parameter for: ${format}`
    );
    
    console.log(`✓ Invalid format rejected: ${format}`);
  }
});
