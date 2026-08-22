class AgySwap < Formula
  desc "Fast account switcher and quota monitor for Google Antigravity CLI"
  homepage "https://github.com/aklkbqx/agy-swap"
  version "2.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.0/agy-swap_v2.1.0_darwin_arm64"
      sha256 "d0bcd9829b75c600a41ab98b55791f8e0596dbd29e98c0f6fe3390c994b678d5"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.0/agy-swap_v2.1.0_darwin_amd64"
      sha256 "9d5926042f959b50413fb2becc9b4687f558412242ba97993189211b331693fb"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.0/agy-swap_v2.1.0_linux_arm64"
      sha256 "5a2850b9b19e4aed505de2d9a152bf4c3bd571b67c69bb09f35f312f13776e6f"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.0/agy-swap_v2.1.0_linux_amd64"
      sha256 "1466f8f2eac37ece384c54b74804d66b456c60200d5ea0bb02e224ed54a82b55"
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
