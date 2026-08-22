class AgySwap < Formula
  desc "Fast account switcher and quota monitor for Google Antigravity CLI"
  homepage "https://github.com/aklkbqx/agy-swap"
  license "MIT"
  head "https://github.com/aklkbqx/agy-swap.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/agy-swap"
  end

  test do
    assert_match "agy-swap v", shell_output("#{bin}/agy-swap --version")
  end
end
