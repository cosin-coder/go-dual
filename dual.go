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

var dualSemaphore = make(chan struct{}, 5)

// ==========================================
// 基础 Builder
// ==========================================
type DualRouteBuilder struct {
	path      string
	method    string
	clientUrl string
	timeout   time.Duration
	tag       string
}

func NewDual(path, method, clientUrl string) *DualRouteBuilder {
	return &DualRouteBuilder{
		path:      path,
		method:    method,
		clientUrl: clientUrl,
		timeout:   3 * time.Second,
		tag:       "未命名接口",
	}
}

func (b *DualRouteBuilder) Timeout(ms int) *DualRouteBuilder {
	b.timeout = time.Duration(ms) * time.Millisecond
	return b
}

func (b *DualRouteBuilder) Tag(name string) *DualRouteBuilder {
	b.tag = name
	return b
}

func (b *DualRouteBuilder) Route(mux *http.ServeMux, handler func(espData string) (any, error)) {
	coreHandle(mux, b, handler)
}

// ==========================================
// 泛型 Builder (链式保持)
// ==========================================
type TypedDualRouteBuilder[T any] struct {
	*DualRouteBuilder
}

func Typed[T any](path, method, clientUrl string) *TypedDualRouteBuilder[T] {
	return &TypedDualRouteBuilder[T]{
		DualRouteBuilder: NewDual(path, method, clientUrl),
	}
}

// 🌟 关键：重写 Tag，强制返回泛型结构体，保持链式不断
func (b *TypedDualRouteBuilder[T]) Tag(name string) *TypedDualRouteBuilder[T] {
	b.DualRouteBuilder.Tag(name)
	return b
}

// 🌟 关键：重写 Timeout，强制返回泛型结构体，保持链式不断
func (b *TypedDualRouteBuilder[T]) Timeout(ms int) *TypedDualRouteBuilder[T] {
	b.DualRouteBuilder.Timeout(ms)
	return b
}

// Handle 方法
func (b *TypedDualRouteBuilder[T]) Handle(mux *http.ServeMux, handler func(espData T) (any, error)) {
	coreHandle(mux, b.DualRouteBuilder, func(rawString string) (any, error) {
		var parsedData T
		if err := json.Unmarshal([]byte(rawString), &parsedData); err != nil {
			return map[string]string{
				"error":   "硬件数据解析失败",
				"details": err.Error(),
				"raw":     rawString,
			}, nil
		}
		return handler(parsedData)
	})
}

// ==========================================
// 底层核心逻辑
// ==========================================
func coreHandle(mux *http.ServeMux, b *DualRouteBuilder, handler func(string) (any, error)) {
	fmt.Printf("📌 注册路由: [%s] %s %s (超时: %v)\n", b.tag, b.method, b.path, b.timeout)

	mux.HandleFunc(b.path, func(w http.ResponseWriter, r *http.Request) {
		dualSemaphore <- struct{}{}
		defer func() { <-dualSemaphore }()

		fullUrl := b.clientUrl + b.path
		if r.URL.RawQuery != "" {
			fullUrl += "?" + r.URL.RawQuery
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()

		var bodyReader io.Reader
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
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
			respBytes, _ := io.ReadAll(resp.Body)
			rawString = string(respBytes)
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