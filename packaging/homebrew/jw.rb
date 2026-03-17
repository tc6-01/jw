class Jw < Formula
  desc "Terminal web jump tool (zoxide-like for URLs)"
  homepage "https://github.com/<owner>/jw"
  url "https://github.com/<owner>/jw/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "<REPLACE_WITH_RELEASE_TARBALL_SHA256>"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/jw"
  end

  test do
    output = shell_output("#{bin}/jw tutorial")
    assert_match "jw 可执行教程", output
  end
end
