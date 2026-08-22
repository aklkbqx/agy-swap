class AgySwap < Formula
  desc "Fast account switcher and quota monitor for Google Antigravity CLI"
  homepage "https://github.com/aklkbqx/agy-swap"
  version "2.1.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.1/agy-swap_v2.1.1_darwin_arm64"
      sha256 "8d2e0da18ae0e13e66347810c460cee247a97f0f71533e2e00c5804ffb2db395"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.1/agy-swap_v2.1.1_darwin_amd64"
      sha256 "8df4a5fac974f7f6230259eb3e38bc149c5448fd93a40cb33fc403f9391027c1"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.1/agy-swap_v2.1.1_linux_arm64"
      sha256 "3e77e16e008c1c8f04739fa5d7bd48f01510a909159ed909b547cbe77e7f35fd"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.1/agy-swap_v2.1.1_linux_amd64"
      sha256 "7c54df267d7d532484e18517b94c34eb3868a993d4d2a2509ee1778326e667c7"
    end
  end

  def install
    binary = Dir["agy-swap_v#{version}_*"].first
    bin.install binary => "agy-swap"
  end

  test do
    assert_match "agy-swap v#{version}", shell_output("#{bin}/agy-swap --version")
  end
end
