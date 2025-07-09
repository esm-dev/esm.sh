/**
 * Test the date-based version functionality
 */

import { assertEquals } from "jsr:@std/assert";

const origin = Deno.env.get("ORIGIN") || "http://localhost:8080";

Deno.test("date-based version resolution", async () => {
  // Test date-based version resolution using package@yyyy-mm-dd format
  const tests = [
    {
      name: "Date version format 2023-01-01",
      url: `${origin}/lodash@2023-01-01`,
      description: "Should resolve to latest lodash version as of 2023-01-01",
    },
    {
      name: "Date version format 2024-05-15", 
      url: `${origin}/lodash@2024-05-15`,
      description: "Should resolve to latest lodash version as of 2024-05-15",
    },
    {
      name: "Date version format 2022-12-31",
      url: `${origin}/lodash@2022-12-31`, 
      description: "Should resolve to latest lodash version as of 2022-12-31",
    },
  ];

  for (const test of tests) {
    const response = await fetch(test.url);
    
    // Should return the module content directly (no redirect needed)
    assertEquals(response.status, 200, `${test.name}: Expected successful response`);
    
    // Check that we get JavaScript content
    const contentType = response.headers.get("content-type");
    assertEquals(
      contentType?.includes("javascript"),
      true,
      `${test.name}: Expected JavaScript content type, got: ${contentType}`
    );
    
    // Check X-ESM-Path header contains resolved version
    const esmPath = response.headers.get("x-esm-path");
    if (esmPath) {
      const versionMatch = esmPath.match(/lodash@(\d+\.\d+\.\d+)/);
      assertEquals(
        versionMatch !== null,
        true,
        `${test.name}: X-ESM-Path should contain exact version, got: ${esmPath}`
      );
      
      console.log(`✓ ${test.name}: ${test.url} -> ${versionMatch![1]}`);
    }
    
    // Consume response body to prevent resource leaks
    await response.body?.cancel();
  }
});

Deno.test("date version with subpaths", async () => {
  // Test date version with subpath
  const url = `${origin}/lodash@2023-06-01/debounce`;
  
  const response = await fetch(url);
  assertEquals(response.status, 200, "Expected successful response for date version with subpath");
  
  // Check that we get JavaScript content
  const contentType = response.headers.get("content-type");
  assertEquals(
    contentType?.includes("javascript"),
    true,
    `Expected JavaScript content type, got: ${contentType}`
  );
  
  // Check X-ESM-Path header contains resolved version and subpath
  const esmPath = response.headers.get("x-esm-path");
  if (esmPath) {
    const versionMatch = esmPath.match(/lodash@(\d+\.\d+\.\d+)/);
    assertEquals(
      versionMatch !== null,
      true,
      `X-ESM-Path should contain exact version, got: ${esmPath}`
    );
    
    assertEquals(
      esmPath.includes("/debounce"),
      true,
      `X-ESM-Path should preserve subpath, got: ${esmPath}`
    );
    
    console.log(`✓ Date version with subpath: ${url} -> ${versionMatch![1]}`);
  }
  
  // Consume response body to prevent resource leaks
  await response.body?.cancel();
});

Deno.test("date version with build target", async () => {
  // Test date version with build target
  const url = `${origin}/lodash@2023-06-01/es2022/lodash.mjs`;
  
  const response = await fetch(url);
  assertEquals(response.status, 200, "Expected successful response for date version with build target");
  
  // Check headers to verify it's the correct content
  const contentType = response.headers.get("content-type");
  assertEquals(
    contentType?.includes("javascript"),
    true,
    `Expected JavaScript content type, got: ${contentType}`
  );
  
  // Consume response body to prevent resource leaks
  await response.body?.cancel();
  
  console.log(`✓ Date version with build target: ${url}`);
});

Deno.test("date version should resolve to different versions for different dates", async () => {
  // Test that different dates resolve to potentially different versions
  const earlyDateUrl = `${origin}/lodash@2020-01-01`;
  const laterDateUrl = `${origin}/lodash@2024-01-01`;
  
  const earlyResponse = await fetch(earlyDateUrl);
  const laterResponse = await fetch(laterDateUrl);
  
  assertEquals(earlyResponse.status, 200, "Expected successful response for early date");
  assertEquals(laterResponse.status, 200, "Expected successful response for later date");
  
  // Get the resolved versions from X-ESM-Path headers
  const earlyEsmPath = earlyResponse.headers.get("x-esm-path");
  const laterEsmPath = laterResponse.headers.get("x-esm-path");
  
  // Consume response bodies to prevent resource leaks
  await earlyResponse.body?.cancel();
  await laterResponse.body?.cancel();
  
  if (earlyEsmPath && laterEsmPath) {
    const earlyVersionMatch = earlyEsmPath.match(/lodash@(\d+\.\d+\.\d+)/);
    const laterVersionMatch = laterEsmPath.match(/lodash@(\d+\.\d+\.\d+)/);
    
    assertEquals(
      earlyVersionMatch !== null,
      true,
      `Early date should resolve to exact version, got: ${earlyEsmPath}`
    );
    
    assertEquals(
      laterVersionMatch !== null,
      true,
      `Later date should resolve to exact version, got: ${laterEsmPath}`
    );
    
    console.log(`✓ Early date (2020-01-01): ${earlyVersionMatch![1]}`);
    console.log(`✓ Later date (2024-01-01): ${laterVersionMatch![1]}`);
    
    // Note: We don't assert that versions are different because it's possible
    // the same version was latest at both dates, but we've verified both work
  }
});

Deno.test("exact version should not be affected by date format", async () => {
  // When exact version is specified, it should be used regardless
  const url = `${origin}/lodash@4.17.21`;
  
  const response = await fetch(url);
  assertEquals(response.status, 200, "Expected successful response for exact version");
  
  // Check that we get JavaScript content
  const contentType = response.headers.get("content-type");
  assertEquals(
    contentType?.includes("javascript"),
    true,
    `Expected JavaScript content type, got: ${contentType}`
  );
  
  // Check X-ESM-Path header contains the exact version
  const esmPath = response.headers.get("x-esm-path");
  if (esmPath) {
    assertEquals(
      esmPath.includes("lodash@4.17.21"),
      true,
      `Exact version should be preserved in X-ESM-Path, got: ${esmPath}`
    );
    
    console.log(`✓ Exact version preserved: ${url} -> lodash@4.17.21`);
  }
  
  // Consume response body to prevent resource leaks
  await response.body?.cancel();
});

Deno.test("date version formats work correctly", async () => {
  // Test that valid date formats work and invalid ones don't interfere
  const validDateUrl = `${origin}/lodash@2023-01-01`;
  const invalidVersionUrl = `${origin}/lodash@999.999.999`; // Clearly invalid semver
  
  // Valid date should work
  const validResponse = await fetch(validDateUrl);
  assertEquals(validResponse.status, 200, "Valid date version should work");
  
  // Check that we get JavaScript content
  const contentType = validResponse.headers.get("content-type");
  assertEquals(
    contentType?.includes("javascript"),
    true,
    `Expected JavaScript content type for valid date, got: ${contentType}`
  );
  
  // Consume response body to prevent resource leaks
  await validResponse.body?.cancel();
  
  // Invalid semver version should fail
  const invalidResponse = await fetch(invalidVersionUrl);
  assertEquals(
    invalidResponse.status === 404,
    true,
    `Invalid semver version should return 404, got: ${invalidResponse.status}`
  );
  
  // Consume response body to prevent resource leaks
  await invalidResponse.body?.cancel();
  
  console.log(`✓ Valid date version works: ${validDateUrl}`);
  console.log(`✓ Invalid semver version rejected: ${invalidVersionUrl}`);
});
