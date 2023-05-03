import { assert } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { Octokit } from "http://localhost:8080/@octokit-next/core@2.5.0";
import "http://localhost:8080/@octokit-next/types-rest-api@2.5.0";

Deno.test("issue #433", async () => {
  const octokit = new Octokit();

  const { data } = await octokit.request("GET /");

  // should be typed as string
  data.current_user_url;
});
