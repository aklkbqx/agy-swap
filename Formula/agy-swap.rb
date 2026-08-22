class AgySwap < Formula
  desc "Fast account switcher and quota monitor for Google Antigravity CLI"
  homepage "https://github.com/aklkbqx/agy-swap"
  version "2.1.2"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.2/agy-swap_v2.1.2_darwin_arm64"
      sha256 "3e47ab1b313a991634dc2843a99be1c5264107e1727e14df70a085000318ad78"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.2/agy-swap_v2.1.2_darwin_amd64"
      sha256 "37891eca59513d1e36d734b2e4b77938e962a955ae598b73b964c4dd049bf49e"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.2/agy-swap_v2.1.2_linux_arm64"
      sha256 "4bea661c03c9ce2d3f59bd16fbc4bf5665b8a5c1f0ccc90948131bbad187afa0"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v2.1.2/agy-swap_v2.1.2_linux_amd64"
      sha256 "d763caa4d4ba849ad0f3da123134b71b8596a7c5746818642bb24ecca27ad5a2"
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
