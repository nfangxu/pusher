package token

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
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
		return nil, fmt.Errorf("invalid base64: %w", err)
	}

	params, err := url.ParseQuery(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid query format: %w", err)
	}

	channel := params.Get("channel")
	group := params.Get("group")
	uuid := params.Get("uuid")
	tsStr := params.Get("ts")
	sign := params.Get("sign")

	if channel == "" || group == "" || uuid == "" || tsStr == "" || sign == "" {
		return nil, fmt.Errorf("missing required fields")
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	now := time.Now().Unix()
	if now-ts > v.expireSeconds {
		return nil, fmt.Errorf("token expired")
	}

	expectedSign := calcSign(channel, group, uuid, ts, v.salt)
	if sign != expectedSign {
		return nil, fmt.Errorf("invalid signature")
	}

	return &Claims{
		Channel: channel,
		Group:   group,
		UUID:    uuid,
		TS:      ts,
	}, nil
}

func calcSign(channel, group, uuid string, ts int64, salt string) string {
	data := fmt.Sprintf("%s%s%s%d%s", channel, group, uuid, ts, salt)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func (c *Claims) UserKey() string {
	return strings.Join([]string{c.Channel, c.Group, c.UUID}, ":")
}
