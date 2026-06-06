package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"testing"
	"time"
)

const testToken = "123456:TEST-bot-token"

// signInitData builds a valid Mini App initData string for the given token.
func signInitData(token, user string, authDate int64) string {
	vals := url.Values{}
	vals.Set("auth_date", strconv.FormatInt(authDate, 10))
	vals.Set("user", user)
	secret := hmacSum([]byte("WebAppData"), []byte(token))
	mac := hmacSum(secret, []byte(dataCheckString(vals, "hash")))
	vals.Set("hash", hex.EncodeToString(mac))
	return vals.Encode()
}

func TestValidateInitData(t *testing.T) {
	user := `{"id":12345,"first_name":"Test","username":"tester"}`
	initData := signInitData(testToken, user, time.Now().Unix())

	got, err := validateInitData(initData, testToken)
	if err != nil {
		t.Fatalf("valid initData rejected: %v", err)
	}
	if got.ID != 12345 || got.Username != "tester" || got.FirstName != "Test" {
		t.Fatalf("unexpected user: %+v", got)
	}

	// Wrong token must fail.
	if _, err := validateInitData(initData, "999:other"); err == nil {
		t.Fatal("expected failure with wrong token")
	}

	// Tampered payload must fail.
	tampered := signInitData(testToken, user, time.Now().Unix())
	tampered = tampered + "&extra=1"
	if _, err := validateInitData(tampered, testToken); err == nil {
		t.Fatal("expected failure for tampered initData")
	}

	// Stale auth_date must fail.
	old := signInitData(testToken, user, time.Now().Add(-48*time.Hour).Unix())
	if _, err := validateInitData(old, testToken); err == nil {
		t.Fatal("expected failure for stale auth_date")
	}
}

func TestValidateLoginWidget(t *testing.T) {
	fields := map[string]string{
		"id":         "777",
		"first_name": "Web",
		"username":   "weblogin",
		"auth_date":  strconv.FormatInt(time.Now().Unix(), 10),
	}
	vals := url.Values{}
	for k, v := range fields {
		vals.Set(k, v)
	}
	sum := sha256.Sum256([]byte(testToken))
	fields["hash"] = hex.EncodeToString(hmacSum(sum[:], []byte(dataCheckString(vals, "hash"))))

	got, err := validateLoginWidget(fields, testToken)
	if err != nil {
		t.Fatalf("valid widget data rejected: %v", err)
	}
	if got.ID != 777 || got.Username != "weblogin" {
		t.Fatalf("unexpected user: %+v", got)
	}

	fields["hash"] = "deadbeef"
	if _, err := validateLoginWidget(fields, testToken); err == nil {
		t.Fatal("expected failure for bad hash")
	}
}
