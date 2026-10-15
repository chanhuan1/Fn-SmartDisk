// Store CF_API_TOKEN + CF_ACCOUNT_ID as GitHub Actions repo secrets.
// Uses sodium sealed-box encryption (libsodium) per GitHub Actions Secrets API.
import { spawnSync } from "node:child_process";

const TOKEN = process.env.GH_TOKEN;
const REPO = "chanhuan1/Fn-SmartDisk";
const CF_TOKEN = process.env.CF_TOKEN;
const CF_ACCOUNT = process.env.CF_ACCOUNT;

if (!TOKEN || !CF_TOKEN || !CF_ACCOUNT) {
  console.error("Usage: GH_TOKEN=... CF_TOKEN=... CF_ACCOUNT=... node set-secrets.mjs");
  process.exit(1);
}

async function ghApi(method, path, body) {
  const res = await fetch(`https://api.github.com${path}`, {
    method,
    headers: {
      Authorization: `token ${TOKEN}`,
      "User-Agent": "codex",
      Accept: "application/vnd.github+json",
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  console.log(`${method} ${path} -> ${res.status}`);
  if (!res.ok) throw new Error(text.slice(0, 500));
  return text ? JSON.parse(text) : null;
}

const pk = await ghApi("GET", `/repos/${REPO}/actions/secrets/public-key`);
console.log("public key:", pk.key_id);

// sodium sealed box: ephemeral X25519 keypair + XSalsa20-Poly1305.
// Node has no native sealed box; shell out to npx tweetnacl-sealedbox-js
// or use libsodium-wrappers via npm install. Simpler: use Python (installed on runner).
// On this dev machine we'll require libsodium-wrappers.
process.stdout.write("installing libsodium-wrappers... ");
const npm = spawnSync("npm", ["install", "libsodium-wrappers", "--no-audit", "--no-fund", "--silent"], {
  stdio: "inherit",
});
if (npm.status !== 0) {
  console.error("npm install failed");
  process.exit(1);
}
const sodium = (await import("libsodium-wrappers")).default;
await sodium.ready;

function encrypt(secret, keyB64) {
  const key = Buffer.from(keyB64, "base64");
  const message = Buffer.from(secret, "utf8");
  const sealed = sodium.crypto_box_seal(message, key);
  return Buffer.from(sealed).toString("base64");
}

await ghApi("PUT", `/repos/${REPO}/actions/secrets/CF_API_TOKEN`, {
  encrypted_value: encrypt(CF_TOKEN, pk.key),
  key_id: pk.key_id,
});
await ghApi("PUT", `/repos/${REPO}/actions/secrets/CF_ACCOUNT_ID`, {
  encrypted_value: encrypt(CF_ACCOUNT, pk.key),
  key_id: pk.key_id,
});

const list = await ghApi("GET", `/repos/${REPO}/actions/secrets`);
console.log("secrets:", list.secrets.map((s) => s.name).join(", "));
