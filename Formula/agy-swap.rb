# Formula for agy-swap by @aklkbqx (https://github.com/aklkbqx)

class AgySwap < Formula
  desc "Minimal interactive account switcher (TUI) for Google Antigravity CLI (agy)"
  homepage "https://github.com/aklkbqx/agy-swap"
  url "https://github.com/aklkbqx/agy-swap/raw/main/agy-swap"
  version "1.0.0"
  sha256 "3b82c4295d4d41e24fb2b0591089d7e2e65dffcd9b6983565fa7a0eb0de61455"

  depends_on "python"

  def install
    bin.install "agy-swap"
  end

  test do
    assert_match "Usage:", shell_output("#{bin}/agy-swap --help")
  end
end
