# 后台推送接入指南

后台服务通过 HTTP POST 调用 `/push` 接口向在线用户推送消息。

---

## 调用方式

```bash
curl -X POST http://your-push-server:8080/push \
  -H "Authorization: Bearer your-push-token-here" \
  -H "Content-Type: application/json" \
  -d '{
    "targets": ["news:admin:u1"],
    "message": {"type": "alert", "content": "你有一条新消息"}
  }'
```

---

## 各语言示例

### Go

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type PushRequest struct {
	Targets []string    `json:"targets"`
	Message interface{} `json:"message"`
}

type PushResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// Push 向推送服务发送消息
func Push(ctx context.Context, serverURL, pushToken string, targets []string, message interface{}) (*PushResponse, error) {
	reqBody, err := json.Marshal(PushRequest{
		Targets: targets,
		Message: message,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL+"/push", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pushToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var result PushResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := Push(ctx, "http://localhost:8080", "your-push-token-here",
		[]string{"news:admin:u1"},
		map[string]interface{}{"type": "alert", "content": "hello"},
	)
	if err != nil {
		fmt.Printf("推送失败: %v\n", err)
		return
	}
	if resp.Code != 0 {
		fmt.Printf("推送错误: code=%d msg=%s\n", resp.Code, resp.Msg)
		return
	}
	fmt.Println("推送成功")
}
```

### PHP（cURL）

```php
<?php

/**
 * 向推送服务发送消息（cURL 版本，无需额外依赖）
 *
 * @param string $serverUrl   推送服务地址，如 http://localhost:8080
 * @param string $pushToken   推送接口认证 Token
 * @param array  $targets     推送目标，如 ["news:*", "news:admin:u1"]
 * @param array  $message     消息体，任意可 JSON 编码的数据
 * @return array 响应数组，包含 code 和 msg 字段
 * @throws RuntimeException 请求失败时抛出异常
 */
function push(string $serverUrl, string $pushToken, array $targets, array $message): array
{
    $body = json_encode([
        'targets' => $targets,
        'message' => $message,
    ], JSON_UNESCAPED_UNICODE);

    $ch = curl_init($serverUrl . '/push');
    curl_setopt_array($ch, [
        CURLOPT_POST           => true,
        CURLOPT_POSTFIELDS     => $body,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_TIMEOUT        => 5,
        CURLOPT_HTTPHEADER     => [
            'Content-Type: application/json',
            'Authorization: Bearer ' . $pushToken,
        ],
    ]);

    $response = curl_exec($ch);
    if (curl_errno($ch)) {
        $error = curl_error($ch);
        curl_close($ch);
        throw new RuntimeException("推送请求失败: {$error}");
    }
    curl_close($ch);

    return json_decode($response, true);
}

// --- 使用示例 ---

try {
    $result = push(
        'http://localhost:8080',
        'your-push-token-here',
        ['news:admin:u1'],
        ['type' => 'alert', 'content' => 'hello']
    );

    if ($result['code'] === 0) {
        echo "推送成功\n";
    } else {
        echo "推送错误: code={$result['code']} msg={$result['msg']}\n";
    }
} catch (RuntimeException $e) {
    echo $e->getMessage() . "\n";
}
```

### PHP（Guzzle）

需要先安装 Guzzle：`composer require guzzlehttp/guzzle`

```php
<?php

require 'vendor/autoload.php';

use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;

/**
 * 向推送服务发送消息（Guzzle 版本，推荐用于已有 Guzzle 依赖的项目）
 *
 * @param string $serverUrl   推送服务地址，如 http://localhost:8080
 * @param string $pushToken   推送接口认证 Token
 * @param array  $targets     推送目标，如 ["news:*", "news:admin:u1"]
 * @param array  $message     消息体，任意可 JSON 编码的数据
 * @return array 响应数组，包含 code 和 msg 字段
 * @throws GuzzleException 请求失败时抛出异常
 */
function push(string $serverUrl, string $pushToken, array $targets, array $message): array
{
    $client = new Client([
        'base_uri' => $serverUrl,
        'timeout'  => 5,
    ]);

    $response = $client->post('/push', [
        'headers' => [
            'Authorization' => 'Bearer ' . $pushToken,
        ],
        'json' => [
            'targets' => $targets,
            'message' => $message,
        ],
    ]);

    return json_decode($response->getBody()->getContents(), true);
}

// --- 使用示例 ---

try {
    $result = push(
        'http://localhost:8080',
        'your-push-token-here',
        ['news:admin:u1'],
        ['type' => 'alert', 'content' => 'hello']
    );

    if ($result['code'] === 0) {
        echo "推送成功\n";
    } else {
        echo "推送错误: code={$result['code']} msg={$result['msg']}\n";
    }
} catch (GuzzleException $e) {
    echo "推送请求失败: " . $e->getMessage() . "\n";
}
```

---

## 推送目标（targets）格式

`targets` 是一个字符串数组，支持以下格式：

| 格式 | 含义 | 示例 |
|------|------|------|
| `*` | 广播给所有在线用户 | `["*"]` |
| `{channel}:*` | 该频道下所有用户 | `["news:*"]` |
| `{channel}:{group}:*` | 该分组下所有用户 | `["news:admin:*"]` |
| `{channel}:{group}:{uuid}` | 指定用户 | `["news:admin:u1"]` |

可以在一次请求中混合使用多种格式，服务端会自动去重，同一连接不会收到重复消息：

```json
{
    "targets": ["news:*", "sports:admin:*", "system:ops:u1"],
    "message": {"type": "system", "content": "服务将于今晚 22:00 维护"}
}
```

---

## 消息体（message）格式

`message` 可以是任意合法 JSON，服务端不做解析，原样推送给前端：

```json
{
    "targets": ["news:admin:*"],
    "message": {
        "type": "notification",
        "title": "新文章发布",
        "content": "《Go 并发编程指南》已发布",
        "url": "https://example.com/articles/123",
        "timestamp": 1716400000
    }
}
```

---

## 推送响应

成功时返回：

```json
{"code": 0, "msg": "ok"}
```

失败时返回对应错误码，参见 [API 接口文档](api.md#错误码一览)。

---

## 最佳实践

1. **批量推送**：一次请求推送多个 target，而不是多次单目标请求
2. **错误重试**：收到 2002（队列满）时稍后重试
3. **限流控制**：推送频率不要超过服务端配置的 `push.rate_limit`
4. **消息体精简**：message 尽量精简，减少网络传输开销
5. **异步调用**：推送是非关键路径，建议后台异步调用，不要阻塞主流程

---

## Token 生成规则

Token 用于前端建立 SSE 或 WebSocket 连接时的身份验证。Token 由后端生成，返回给前端。

### 格式

```
base64(channel={channel}&group={group}&uuid={uuid}&ts={unix_timestamp}&sign={HMAC-SHA256_sign})
```

### 签名算法

```
sign = hex(hmac_sha256(secret=salt, message={channel}{group}{uuid}{ts}))
```

注意：签名消息中各字段值**直接拼接，无分隔符**，salt 作为 HMAC secret 使用。

### 生成示例

以 `channel=news`、`group=admin`、`uuid=u1`、`ts=1747830600`、`salt=mysecret` 为例：

```
签名消息: "newsadminu11747830600"
HMAC-SHA256: "a1b2c3d4e5f6..."
Token:       base64("channel=news&group=admin&uuid=u1&ts=1747830600&sign=a1b2c3d4e5f6...")
```

### 各语言 Token 生成代码

#### Go

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

// GenerateToken 生成连接用的 Token
// salt: 签名盐值，需与服务端 config.yaml 中 token.salt 一致
// channel, group, uuid: 用户标识三元组
func GenerateToken(salt, channel, group, uuid string) string {
	ts := time.Now().Unix()
	signInput := fmt.Sprintf("%s%s%s%d", channel, group, uuid, ts)
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(signInput))
	sign := fmt.Sprintf("%x", mac.Sum(nil))
	token := fmt.Sprintf("channel=%s&group=%s&uuid=%s&ts=%d&sign=%s", channel, group, uuid, ts, sign)
	return base64.StdEncoding.EncodeToString([]byte(token))
}

func main() {
	token := GenerateToken("your-secret-salt-here", "news", "admin", "u1")
	fmt.Println(token)
}
```

#### PHP

```php
<?php

/**
 * 生成连接用的 Token
 *
 * @param string $salt    签名盐值，需与服务端 config.yaml 中 token.salt 一致
 * @param string $channel 业务频道
 * @param string $group   用户分组
 * @param string $uuid    用户唯一标识
 * @return string Base64 编码的 Token
 */
function generateToken(string $salt, string $channel, string $group, string $uuid): string
{
    $ts = time();
    $signInput = $channel . $group . $uuid . $ts;
    $sign = hash_hmac('sha256', $signInput, $salt);
    $token = "channel={$channel}&group={$group}&uuid={$uuid}&ts={$ts}&sign={$sign}";
    return base64_encode($token);
}

// --- 使用示例 ---

$token = generateToken('your-secret-salt-here', 'news', 'admin', 'u1');
echo $token . "\n";
```

### 注意事项

1. `salt` 必须与服务端配置的 `token.salt` 一致
2. `ts` 为 Unix 秒级时间戳，服务端会校验是否在 `token.expire_seconds` 范围内
3. Token 过期后前端需重新获取，建议在后端提供一个接口返回新 Token
4. **不要在前端生成 Token**，salt 不应暴露给客户端
