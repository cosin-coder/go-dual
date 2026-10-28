package dual

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 内置协程背压：限制最多同时 5 个请求访问 ESP32
var dualSemaphore = make(chan struct{}, 5)

type DualRouteBuilder struct {
	path      string
	method    string
	clientUrl string
	timeout   time.Duration
	tag       string
}

// 构造函数，相当于 Kotlin 的 Route.Dual(...)
func NewDual(path, method, clientUrl string) *DualRouteBuilder {
	return &DualRouteBuilder{
		path:      path,
		method:    method,
		clientUrl: clientUrl,
		timeout:   3 * time.Second,
		tag:       "未命名接口",
	}
}

// 链式配置：超时时间
func (b *DualRouteBuilder) Timeout(ms int) *DualRouteBuilder {
	b.timeout = time.Duration(ms) * time.Millisecond
	return b
}

// 链式配置：标签
func (b *DualRouteBuilder) Tag(name string) *DualRouteBuilder {
	b.tag = name
	return b
}

// 泛型版本（支持自动反序列化）
func GetTyped[T any](b *DualRouteBuilder, handler func(espData T) (any, error)) {
	b.handle(func(rawString string) (any, error) {
		var parsedData T
		if err := json.Unmarshal([]byte(rawString), &parsedData); err != nil {
			return map[string]string{
				"error":    "硬件数据反序列化失败",
				"details":  err.Error(),
				"raw_data": rawString,
			}, nil
		}
		return handler(parsedData)
	})
}

// String 版本（支持网页拉取、纯文本）
func (b *DualRouteBuilder) Get(handler func(espData string) (any, error)) {
	b.handle(handler)
}

func (b *DualRouteBuilder) handle(handler func(rawString string) (any, error)) {
	fmt.Printf("📌 注册路由: [%s] %s %s (超时: %v)\n", b.tag, b.method, b.path, b.timeout)

	http.HandleFunc(b.path, func(w http.ResponseWriter, r *http.Request) {
		dualSemaphore <- struct{}{}
		defer func() { <-dualSemaphore }()

		fullUrl := b.clientUrl + b.path
		if r.URL.RawQuery != "" {
			fullUrl += "?" + r.URL.RawQuery
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()

		var bodyReader io.Reader
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			bodyBytes, _ := io.ReadAll(r.Body)
			bodyReader = bytes.NewBuffer(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, b.method, fullUrl, bodyReader)
		if err != nil {
			http.Error(w, "构建请求失败", http.StatusInternalServerError)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		
		var rawString string
		if err != nil {
			rawString = fmt.Sprintf(`{"error": "无法连接硬件: %v"}`, err)
		} else {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			rawString = string(bodyBytes)
		}

		result, err := handler(rawString)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		switch v := result.(type) {
		case string:
			if strings.HasPrefix(strings.TrimSpace(v), "<") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			} else {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			}
			w.Write([]byte(v))
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(w).Encode(result)
		}
	})
}