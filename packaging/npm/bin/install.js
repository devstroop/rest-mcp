#!/usr/bin/env node

/**
 * rest-mcp npm wrapper — downloads the correct pre-built binary
 * for the current platform on postinstall.
 */

const { execSync } = require("child_process");
const fs = require("fs");
const https = require("https");
const path = require("path");
const os = require("os");
const zlib = require("zlib");

const pkg = require("../package.json");
const VERSION = pkg.version;
const REPO = "devstroop/rest-mcp";

function getPlatformInfo() {
  const platform = os.platform();
  const arch = os.arch();

  const platformMap = {
    linux: "linux",
    darwin: "darwin",
    win32: "windows",
  };

  const archMap = {
    x64: "amd64",
    arm64: "arm64",
  };

  const goos = platformMap[platform];
  const goarch = archMap[arch];

  if (!goos || !goarch) {
    throw new Error(
      `Unsupported platform: ${platform}/${arch}. ` +
        `Supported: linux/darwin/windows on amd64/arm64.`
    );
  }

  const ext = platform === "win32" ? "zip" : "tar.gz";
  return { goos, goarch, ext };
}

function downloadFile(url) {
  return new Promise((resolve, reject) => {
    const request = (url) => {
      https
        .get(url, (res) => {
          if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            request(res.headers.location);
            return;
          }
          if (res.statusCode !== 200) {
            reject(new Error(`HTTP ${res.statusCode} downloading ${url}`));
            return;
          }
          const chunks = [];
          res.on("data", (chunk) => chunks.push(chunk));
          res.on("end", () => resolve(Buffer.concat(chunks)));
          res.on("error", reject);
        })
        .on("error", reject);
    };
    request(url);
  });
}

async function extractTarGz(buffer, destDir) {
  const tmpFile = path.join(os.tmpdir(), `rest-mcp-${Date.now()}.tar.gz`);
  fs.writeFileSync(tmpFile, buffer);
  try {
    execSync(`tar xzf "${tmpFile}" -C "${destDir}"`, { stdio: "pipe" });
  } finally {
    fs.unlinkSync(tmpFile);
  }
}

async function extractZip(buffer, destDir) {
  const tmpFile = path.join(os.tmpdir(), `rest-mcp-${Date.now()}.zip`);
  fs.writeFileSync(tmpFile, buffer);
  try {
    if (os.platform() === "win32") {
      execSync(
        `powershell -Command "Expand-Archive -Path '${tmpFile}' -DestinationPath '${destDir}' -Force"`,
        { stdio: "pipe" }
      );
    } else {
      execSync(`unzip -o "${tmpFile}" -d "${destDir}"`, { stdio: "pipe" });
    }
  } finally {
    fs.unlinkSync(tmpFile);
  }
}

async function main() {
  const { goos, goarch, ext } = getPlatformInfo();
  const archiveName = `rest-mcp_${VERSION}_${goos}_${goarch}.${ext}`;
  const url = `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;

  console.log(`Downloading rest-mcp v${VERSION} for ${goos}/${goarch}...`);

  const buffer = await downloadFile(url);
  const binDir = path.join(__dirname);

  if (ext === "tar.gz") {
    await extractTarGz(buffer, binDir);
  } else {
    await extractZip(buffer, binDir);
  }

  // Make binary executable on Unix
  const binaryName = goos === "windows" ? "rest-mcp.exe" : "rest-mcp";
  const binaryPath = path.join(binDir, binaryName);

  if (fs.existsSync(binaryPath) && os.platform() !== "win32") {
    fs.chmodSync(binaryPath, 0o755);
  }

  console.log(`rest-mcp v${VERSION} installed successfully.`);
}

main().catch((err) => {
  console.error(`Failed to install rest-mcp: ${err.message}`);
  console.error(
    "You can download the binary manually from https://github.com/devstroop/rest-mcp/releases"
  );
  process.exit(1);
});
