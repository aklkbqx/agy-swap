# Formula for agy-swap by @aklkbqx (https://github.com/aklkbqx)

class AgySwap < Formula
  desc "Minimal interactive account switcher (TUI) for Google Antigravity CLI (agy)"
  homepage "https://github.com/aklkbqx/agy-swap"
  url "https://github.com/aklkbqx/agy-swap/raw/v1.8.2/agy-swap"
  version "1.8.2"
  sha256 "b1123d6d55c5abf73ced5a249700e517f75656faf829be6d201e494ff8070407"

  depends_on "python"

  def install
    bin.install "agy-swap"
  end

  test do
    assert_match "usage: agy-swap", shell_output("#{bin}/agy-swap --help")
  end
end
