package cache

import "testing"

func TestHashKeyDeterministic(t *testing.T) {
	image := "abc123"
	prompt := "answer briefly"

	first := HashKey(image, prompt)
	second := HashKey(image, prompt)

	if first != second {
		t.Fatalf("HashKey returned different values: %q != %q", first, second)
	}
}

func TestHashKeyChangesWithInput(t *testing.T) {
	base := HashKey("image", "prompt")

	if got := HashKey("image", "different prompt"); got == base {
		t.Fatal("HashKey did not change when prompt changed")
	}
	if got := HashKey("different image", "prompt"); got == base {
		t.Fatal("HashKey did not change when image changed")
	}
}

func TestHashKeyHandlesEmptyAndLargeImages(t *testing.T) {
	if got := HashKey("", "prompt"); got == "" {
		t.Fatal("HashKey returned empty key for empty image")
	}

	large := make([]byte, 5000)
	for i := range large {
		large[i] = byte('a' + i%26)
	}
	if got := HashKey(string(large), "prompt"); got == "" {
		t.Fatal("HashKey returned empty key for large image")
	}
}
