package provider

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MaxRetries 是 SendWithRetry 在初始尝试之后的最大重试次数（总共 MaxRetries+1 次尝试）
const MaxRetries = 10

const maxBackoff = 15 * time.Second

// maxAuthRetries 对于曾经成功认证的 key，遇到瞬时 401/403 时的最大重试次数
const maxAuthRetries = 2

// SendOptions 携带 SendWithRetry 所需的每请求上下文
type SendOptions struct {
	Provider  string // provider 实例名，用于错误消息
	KeyEnv    string // API key 的环境变量名
	HasKey    bool   // 是否设置了非空 key
	RetryAuth bool   // key 曾经认证成功 → 瞬时 401 可重试
}

// RetryInfo 描述即将发生的退避：Attempt 是从 1 开始的重试序号
type RetryInfo struct {
	Attempt int
	Max     int
	Delay   time.Duration
	Err     error
}

// RetryNotify 在每次退避等待前被调用，可用于 UI 展示重试状态
type RetryNotify func(RetryInfo)

type retryNotifyKey struct{}

// WithRetryNotify 在 context 中附加重试通知回调
func WithRetryNotify(ctx context.Context, fn RetryNotify) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, retryNotifyKey{}, fn)
}

func retryNotifyFromContext(ctx context.Context) RetryNotify {
	fn, _ := ctx.Value(retryNotifyKey{}).(RetryNotify)
	return fn
}

// RetryableStatus 判断 HTTP 状态码是否值得退避重试：
// 408（超时）、429（限流）、5xx（服务端错误，含 Anthropic 的 529 过载）
func RetryableStatus(s int) bool {
	return s == http.StatusRequestTimeout ||
		s == http.StatusTooManyRequests ||
		(s >= 500 && s <= 599)
}

func transientErr(err error) bool {
	if err == nil {
		return false
	}
	if ctx := context.Canceled; err == ctx {
		return false
	}
	if err == context.DeadlineExceeded {
		return false
	}
	return true
}

// backoffDelay 计算第 attempt 次重试的等待时间。
// 指数退避：500ms × 2^(attempt-1) + jitter，上限 maxBackoff。
// 若服务端返回 Retry-After，优先使用。
func backoffDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > maxBackoff {
			return maxBackoff
		}
		return retryAfter
	}
	d := time.Duration(1<<(attempt-1)) * 500 * time.Millisecond
	if d > maxBackoff {
		d = maxBackoff
	}
	return d + time.Duration(rand.Intn(250))*time.Millisecond
}

func parseRetryAfter(resp *http.Response) time.Duration {
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// SendWithRetry 发起 HTTP POST 流式请求，对连接+头阶段做自动重试。
//
// 重试策略：
//   - 瞬时网络错误 → 退避重试
//   - 408/429/5xx → 退避重试
//   - 401/403 且 RetryAuth → 退避重试最多 maxAuthRetries 次
//   - 其它 4xx → 立即返回 *APIError
//   - context 取消 → 立即返回
//
// 不重试流体阶段：一旦 200 OK 返回，后续 body 中的错误不在此函数处理。
func SendWithRetry(ctx context.Context, httpClient *http.Client, opts SendOptions,
	newReq func(context.Context) (*http.Request, error)) (*http.Response, error) {

	notify := retryNotifyFromContext(ctx)
	var lastErr error
	var retryAfter time.Duration
	authRetries := 0

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoffDelay(attempt, retryAfter)
			if notify != nil {
				notify(RetryInfo{Attempt: attempt, Max: MaxRetries, Delay: delay, Err: lastErr})
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		retryAfter = 0

		req, err := newReq(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: 构建请求失败: %w", opts.Provider, err)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			if !transientErr(err) {
				return nil, fmt.Errorf("%s: 请求失败: %w", opts.Provider, err)
			}
			lastErr = fmt.Errorf("%s: 请求失败: %w", opts.Provider, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		retryAfter = parseRetryAfter(resp)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			authErr := &AuthError{
				Provider: opts.Provider,
				KeyEnv:   opts.KeyEnv,
				Status:   resp.StatusCode,
				HasKey:   opts.HasKey,
			}
			if opts.RetryAuth && authRetries < maxAuthRetries {
				authRetries++
				lastErr = authErr
				continue
			}
			return nil, authErr
		}

		apiErr := &APIError{
			Provider: opts.Provider,
			Status:   resp.StatusCode,
			Body:     strings.TrimSpace(string(msg)),
		}
		if !RetryableStatus(resp.StatusCode) {
			return nil, apiErr
		}
		lastErr = apiErr
	}
	return nil, lastErr
}
