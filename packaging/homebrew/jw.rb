class Jw < Formula
  desc "Terminal web jump tool (zoxide-like for URLs)"
  homepage "https://github.com/tc6-01/jw"
  url "https://github.com/tc6-01/jw/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "3f1368b235baf94c481f387bac1366d07f5e0b0d2da4fd6bbce73d8af3a0217c"
  license "MIT"
  head "https://github.com/tc6-01/jw.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/jw"
  end

  test do
    output = shell_output("#{bin}/jw tutorial")
    assert_match "jw 可执行教程", output
  end
end
