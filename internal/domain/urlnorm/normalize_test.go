package urlnorm

import "testing"

func TestNormalizeAndRedact_RemovesSensitiveQueryAndFragment(t *testing.T) {
	in := "HTTPS://Example.com/path?token=abc123&x=1#frag"
	got, err := NormalizeAndRedact(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "https://example.com/path?token=%3Credacted%3E&x=1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeAndRedact_MasksSensitivePath(t *testing.T) {
	in := "https://example.com/oauth/secret-code?ok=1"
	got, err := NormalizeAndRedact(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "https://example.com/oauth/%3Credacted%3E?ok=1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeAndRedact_BlocksDangerousScheme(t *testing.T) {
	_, err := NormalizeAndRedact("javascript:alert(1)")
	if err != ErrDangerousScheme {
		t.Fatalf("got err %v, want %v", err, ErrDangerousScheme)
	}
}
