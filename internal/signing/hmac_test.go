package signing

import "testing"

func TestSignAndVerify(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	secret := "my-secret-key"

	sig := Sign(payload, secret)

	if sig == "" {
		t.Fatal("signature should not be empty")
	}
	if sig[:7] != "sha256=" {
		t.Fatalf("signature should start with sha256=, got %s", sig[:7])
	}

	if !Verify(payload, secret, sig) {
		t.Fatal("Verify should return true for valid signature")
	}

	if Verify(payload, "wrong-secret", sig) {
		t.Fatal("Verify should return false for wrong secret")
	}

	if Verify([]byte("tampered"), secret, sig) {
		t.Fatal("Verify should return false for tampered payload")
	}
}
