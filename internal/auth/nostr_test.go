package auth

import (
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

const (
	testPrivateKey = "9a9787e3e31a4b0e7e483ed97b1ab0a45534675b07003a51c0840d6a681ad53a"
	testPubkey     = "aa3701983efe79f6eebffc4dc4ef56a2b938beccd19a36830678a7eacadb2052"
)

func makeValidNIP98Event(t *testing.T, url, method string) *nostr.Event {
	t.Helper()
	event := &nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: TimestampFromTime(time.Now()),
		Tags: nostr.Tags{
			{"u", url},
			{"method", method},
		},
		Content: "",
	}
	if err := event.Sign(testPrivateKey); err != nil {
		t.Fatalf("failed to sign event: %v", err)
	}
	return event
}

func TestVerifyNIP98Event_Valid(t *testing.T) {
	t.Parallel()

	url := "http://localhost:8080/auth/verify"
	event := makeValidNIP98Event(t, url, "POST")

	err := VerifyNIP98Event(event, url, "POST")
	if err != nil {
		t.Fatalf("expected valid event, got error: %v", err)
	}
}

func TestVerifyNIP98Event_WrongKind(t *testing.T) {
	t.Parallel()

	url := "http://localhost:8080/auth/verify"
	event := makeValidNIP98Event(t, url, "POST")
	event.Kind = 1

	err := VerifyNIP98Event(event, url, "POST")
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
}

func TestVerifyNIP98Event_NonEmptyContent(t *testing.T) {
	t.Parallel()

	event := &nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: TimestampFromTime(time.Now()),
		Tags: nostr.Tags{
			{"u", "http://localhost:8080/auth/verify"},
			{"method", "POST"},
		},
		Content: "not empty",
	}
	_ = event.Sign(testPrivateKey)

	err := VerifyNIP98Event(event, "http://localhost:8080/auth/verify", "POST")
	if err == nil {
		t.Fatal("expected error for non-empty content")
	}
}

func TestVerifyNIP98Event_URLMismatch(t *testing.T) {
	t.Parallel()

	event := makeValidNIP98Event(t, "http://localhost:8080/auth/verify", "POST")

	err := VerifyNIP98Event(event, "http://evil.com/auth/verify", "POST")
	if err == nil {
		t.Fatal("expected error for URL mismatch")
	}
}

func TestVerifyNIP98Event_MethodMismatch(t *testing.T) {
	t.Parallel()

	event := makeValidNIP98Event(t, "http://localhost:8080/auth/verify", "POST")

	err := VerifyNIP98Event(event, "http://localhost:8080/auth/verify", "GET")
	if err == nil {
		t.Fatal("expected error for method mismatch")
	}
}

func TestVerifyNIP98Event_MethodCaseInsensitive(t *testing.T) {
	t.Parallel()

	event := makeValidNIP98Event(t, "http://localhost:8080/auth/verify", "post")

	err := VerifyNIP98Event(event, "http://localhost:8080/auth/verify", "POST")
	if err != nil {
		t.Fatalf("expected case-insensitive method match, got error: %v", err)
	}
}

func TestVerifyNIP98Event_ExpiredTimestamp(t *testing.T) {
	t.Parallel()

	event := &nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: TimestampFromTime(time.Now().Add(-2 * time.Minute)),
		Tags: nostr.Tags{
			{"u", "http://localhost:8080/auth/verify"},
			{"method", "POST"},
		},
		Content: "",
	}
	_ = event.Sign(testPrivateKey)

	err := VerifyNIP98Event(event, "http://localhost:8080/auth/verify", "POST")
	if err == nil {
		t.Fatal("expected error for expired timestamp")
	}
}

func TestVerifyNIP98Event_FutureTimestamp(t *testing.T) {
	t.Parallel()

	event := &nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: TimestampFromTime(time.Now().Add(2 * time.Minute)),
		Tags: nostr.Tags{
			{"u", "http://localhost:8080/auth/verify"},
			{"method", "POST"},
		},
		Content: "",
	}
	_ = event.Sign(testPrivateKey)

	err := VerifyNIP98Event(event, "http://localhost:8080/auth/verify", "POST")
	if err == nil {
		t.Fatal("expected error for future timestamp")
	}
}

func TestVerifyNIP98Event_MissingUTag(t *testing.T) {
	t.Parallel()

	event := &nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: TimestampFromTime(time.Now()),
		Tags: nostr.Tags{
			{"method", "POST"},
		},
		Content: "",
	}
	_ = event.Sign(testPrivateKey)

	err := VerifyNIP98Event(event, "http://localhost:8080/auth/verify", "POST")
	if err == nil {
		t.Fatal("expected error for missing u tag")
	}
}

func TestVerifyNIP98Event_MissingMethodTag(t *testing.T) {
	t.Parallel()

	event := &nostr.Event{
		Kind:      KindHTTPAuth,
		CreatedAt: TimestampFromTime(time.Now()),
		Tags: nostr.Tags{
			{"u", "http://localhost:8080/auth/verify"},
		},
		Content: "",
	}
	_ = event.Sign(testPrivateKey)

	err := VerifyNIP98Event(event, "http://localhost:8080/auth/verify", "POST")
	if err == nil {
		t.Fatal("expected error for missing method tag")
	}
}

func TestNormalizePubkey_Hex(t *testing.T) {
	t.Parallel()

	hex := "ddc035a6cb2bd5a56a0076e78f2c0de88c2bd24ea0b62acd5acc82b24a52624a"
	result, err := NormalizePubkey(hex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != hex {
		t.Errorf("expected %s, got %s", hex, result)
	}
}

func TestNormalizePubkey_Npub(t *testing.T) {
	t.Parallel()

	npub := "npub14gmsrxp7leuldm4ll3xufm6k52un30kv6xdrdqcx0zn74jkmypfqe60jpx"
	result, err := NormalizePubkey(npub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != testPubkey {
		t.Errorf("expected hex pubkey, got %s", result)
	}
}

func TestNormalizePubkey_Invalid(t *testing.T) {
	t.Parallel()

	_, err := NormalizePubkey("short")
	if err == nil {
		t.Fatal("expected error for invalid pubkey")
	}
}

func TestNormalizePubkeys(t *testing.T) {
	t.Parallel()

	hex := testPubkey
	npub := "npub14gmsrxp7leuldm4ll3xufm6k52un30kv6xdrdqcx0zn74jkmypfqe60jpx"

	result, err := NormalizePubkeys([]string{hex, npub})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0] != hex {
		t.Errorf("expected hex unchanged, got %s", result[0])
	}
	if result[1] != hex {
		t.Errorf("expected npub to decode to same hex, got %s", result[1])
	}
}

func TestNormalizePubkeys_InvalidFails(t *testing.T) {
	t.Parallel()

	_, err := NormalizePubkeys([]string{testPubkey, "bad"})
	if err == nil {
		t.Fatal("expected error for invalid pubkey in list")
	}
}

func TestIsPubkeyAllowed(t *testing.T) {
	t.Parallel()

	allowed := []string{"aaa", "bbb", "ccc"}

	if !IsPubkeyAllowed("bbb", allowed) {
		t.Error("expected bbb to be allowed")
	}

	if IsPubkeyAllowed("ddd", allowed) {
		t.Error("expected ddd to not be allowed")
	}
}
