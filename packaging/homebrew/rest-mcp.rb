# This is a template formula. GoReleaser auto-generates the real one
# in the devstroop/homebrew-tap repo on each release.
#
# Manual setup (if not using GoReleaser):
#   1. Create repo: devstroop/homebrew-tap
#   2. Copy this file to Formula/rest-mcp.rb
#   3. Update url, sha256, and version on each release
#
# Users install via:
#   brew tap devstroop/tap
#   brew install rest-mcp

class RestMcp < Formula
  desc "Turn any REST API into MCP tools — via OpenAPI spec or manual config"
  homepage "https://github.com/devstroop/rest-mcp"
  license "MIT"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/devstroop/rest-mcp/releases/download/v#{version}/rest-mcp_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/devstroop/rest-mcp/releases/download/v#{version}/rest-mcp_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/devstroop/rest-mcp/releases/download/v#{version}/rest-mcp_#{version}_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/devstroop/rest-mcp/releases/download/v#{version}/rest-mcp_#{version}_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  def install
    bin.install "rest-mcp"
  end

  test do
    system "#{bin}/rest-mcp", "--version"
  end
end
