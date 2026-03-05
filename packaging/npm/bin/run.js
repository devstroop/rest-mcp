#!/usr/bin/env node

/**
 * rest-mcp npm wrapper — runs the pre-built binary with all arguments forwarded.
 * Usage: npx rest-mcp --spec openapi.json --base-url https://api.example.com
 */

const { execFileSync } = require("child_process");
const path = require("path");
const os = require("os");
const fs = require("fs");

const binaryName = os.platform() === "win32" ? "rest-mcp.exe" : "rest-mcp";
const binaryPath = path.join(__dirname, binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(
    "rest-mcp binary not found. Run 'npm install' or download from https://github.com/devstroop/rest-mcp/releases"
  );
  process.exit(1);
}

try {
  execFileSync(binaryPath, process.argv.slice(2), {
    stdio: "inherit",
  });
} catch (err) {
  if (err.status !== undefined) {
    process.exit(err.status);
  }
  throw err;
}
