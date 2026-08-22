class AgySwap < Formula
  desc "Fast account switcher and quota monitor for Google Antigravity CLI"
  homepage "https://github.com/aklkbqx/agy-swap"
  version "2.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.0.0/agy-swap_v2.0.0_darwin_arm64"
      sha256 "7453131f672c9d35fd2fd718695096515db15da333a4846315f5ff708fd2d9f2"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.0.0/agy-swap_v2.0.0_darwin_amd64"
      sha256 "0bbb80f35b9a7437ed6ca01e2f9d2983dee395517bbcc55fc1725027d3aaa648"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.0.0/agy-swap_v2.0.0_linux_arm64"
      sha256 "47ddaee558f7d5b0254ca8511c5b3d721afdf782cb3c4cb3557212fa7c6501cf"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.0.0/agy-swap_v2.0.0_linux_amd64"
      sha256 "d29c8aa0ccef150a4df6657b93cf89f3519cee2416c978aadf38bd9c2b5af29c"
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
