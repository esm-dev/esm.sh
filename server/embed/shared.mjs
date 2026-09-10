// Shared browser helpers for the esm.sh static pages.

// solvePow brute-forces the nonce whose SHA-256 prefix (over `salt + nonce`)
// has `difficulty` leading zero hex chars.
export async function solvePow(salt, difficulty) {
  const target = "0".repeat(difficulty);
  const encoder = new TextEncoder();
  for (let nonce = 0; ; nonce++) {
    const bytes = new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(salt + nonce)));
    let hex = "";
    for (const byte of bytes) hex += byte.toString(16).padStart(2, "0");
    if (hex.startsWith(target)) return String(nonce);
  }
}

// fetchPowChallenge asks the server for a fresh challenge of the given scope.
export async function fetchPowChallenge(scope) {
  const response = await fetch("/pow/challenge?scope=" + encodeURIComponent(scope));
  if (!response.ok) {
    throw new Error("failed to fetch proof-of-work challenge (" + response.status + ")");
  }
  return response.json();
}
