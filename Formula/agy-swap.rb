# Formula for agy-swap by @aklkbqx (https://github.com/aklkbqx)

class AgySwap < Formula
  desc "Minimal interactive account switcher (TUI) for Google Antigravity CLI (agy)"
  homepage "https://github.com/aklkbqx/agy-swap"
  url "https://github.com/aklkbqx/agy-swap/raw/main/agy-swap"
  version "1.8.0"
  sha256 "e5b7152710baabb06f4f7daa4ac3cab63b6336d2a29ef4762c589ede2775c61b"

  depends_on "python"

  def install
    bin.install "agy-swap"
  end

  test do
    assert_match "Usage:", shell_output("#{bin}/agy-swap --help")
  end
end
