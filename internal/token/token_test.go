package token

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

func TestGenerate_Validate(t *testing.T) {
	salt := "test-salt"
	channel := "news"
	group := "admin"
	uuid := "u123"

	tokenStr := Generate(salt, channel, group, uuid)
	if tokenStr == "" {
		t.Fatal("Generate() returned empty string")
	}

	v := NewValidator(salt, 3600)
	claims, err := v.Validate(tokenStr)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if claims.Channel != channel {
		t.Errorf("Channel = %v, want %v", claims.Channel, channel)
	}
	if claims.Group != group {
		t.Errorf("Group = %v, want %v", claims.Group, group)
	}
	if claims.UUID != uuid {
		t.Errorf("UUID = %v, want %v", claims.UUID, uuid)
	}

	expectedKey := "news:admin:u123"
	if claims.UserKey() != expectedKey {
		t.Errorf("UserKey() = %v, want %v", claims.UserKey(), expectedKey)
	}
}

func TestValidate_InvalidBase64(t *testing.T) {
	v := NewValidator("salt", 3600)
	_, err := v.Validate("invalid-base64!!!")
	if err == nil {
		t.Error("Validate() expected error for invalid base64")
	}
}

func TestValidate_MissingFields(t *testing.T) {
	v := NewValidator("salt", 3600)
	_, err := v.Validate("Y2hhbm5lbD1uZXdz")
	if err == nil {
		t.Error("Validate() expected error for missing fields")
	}
}

func TestValidate_ExpiredToken(t *testing.T) {
	salt := "test-salt"
	channel := "news"
	group := "admin"
	uuid := "u123"

	ts := time.Now().Unix() - 7200
	sign := calcSign(channel, group, uuid, ts, salt)
	tokenStr := base64Encode("channel=%s&group=%s&uuid=%s&ts=%d&sign=%s", channel, group, uuid, ts, sign)

	v := NewValidator(salt, 3600)
	_, err := v.Validate(tokenStr)
	if err == nil {
		t.Error("Validate() expected error for expired token")
	}
}

func TestValidate_InvalidSignature(t *testing.T) {
	salt := "test-salt"
	channel := "news"
	group := "admin"
	uuid := "u123"

	ts := time.Now().Unix()
	tokenStr := base64Encode("channel=%s&group=%s&uuid=%s&ts=%d&sign=invalid", channel, group, uuid, ts)

	v := NewValidator(salt, 3600)
	_, err := v.Validate(tokenStr)
	if err == nil {
		t.Error("Validate() expected error for invalid signature")
	}
}

func base64Encode(format string, args ...interface{}) string {
	data := fmt.Sprintf(format, args...)
	return base64.StdEncoding.EncodeToString([]byte(data))
}
