package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrTokenExpired       = fmt.Errorf("token expired")
	ErrTokenNotYetValid   = fmt.Errorf("token not yet valid")
	ErrInvalidSignature   = fmt.Errorf("invalid signature")
	ErrMissingFields      = fmt.Errorf("missing required fields")
	ErrInvalidBase64      = fmt.Errorf("invalid base64")
	ErrInvalidQueryFormat = fmt.Errorf("invalid query format")
	ErrInvalidTimestamp   = fmt.Errorf("invalid timestamp")
	ErrInvalidIdentity    = fmt.Errorf("invalid identity")
)

type Claims struct {
	Channel string
	Group   string
	UUID    string
	TS      int64
}

type Validator struct {
	salt          string
	expireSeconds int64
}

func NewValidator(salt string, expireSeconds int) *Validator {
	return &Validator{
		salt:          salt,
		expireSeconds: int64(expireSeconds),
	}
}

func Generate(salt string, channel, group, uuid string) string {
	ts := time.Now().Unix()
	sign := calcSign(channel, group, uuid, ts, salt)
	return base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("channel=%s&group=%s&uuid=%s&ts=%d&sign=%s", channel, group, uuid, ts, sign)),
	)
}

func (v *Validator) Validate(token string) (*Claims, error) {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidBase64
	}

	params, err := url.ParseQuery(string(data))
	if err != nil {
		return nil, ErrInvalidQueryFormat
	}

	channel := params.Get("channel")
	group := params.Get("group")
	uuid := params.Get("uuid")
	tsStr := params.Get("ts")
	sign := params.Get("sign")

	if channel == "" || group == "" || uuid == "" || tsStr == "" || sign == "" {
		return nil, ErrMissingFields
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidTimestamp
	}

	if !validIdentityPart(channel) || !validIdentityPart(group) || !validIdentityPart(uuid) {
		return nil, ErrInvalidIdentity
	}

	now := time.Now().Unix()
	if ts-now > 60 {
		return nil, ErrTokenNotYetValid
	}
	if now-ts > v.expireSeconds {
		return nil, ErrTokenExpired
	}

	expectedSign := calcSign(channel, group, uuid, ts, v.salt)
	if !hmac.Equal([]byte(sign), []byte(expectedSign)) {
		return nil, ErrInvalidSignature
	}

	return &Claims{
		Channel: channel,
		Group:   group,
		UUID:    uuid,
		TS:      ts,
	}, nil
}

func calcSign(channel, group, uuid string, ts int64, salt string) string {
	data := fmt.Sprintf("%s%s%s%d", channel, group, uuid, ts)
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(data))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func validIdentityPart(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func (c *Claims) UserKey() string {
	return strings.Join([]string{c.Channel, c.Group, c.UUID}, ":")
}
