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

type Router interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// ==========================================
// Configuration Struct
// ==========================================
type DualConfig struct {
	Path      string
	Method    string
	ClientURL string
}

// ==========================================
// Base Builder
// ==========================================
type DualRouteBuilder struct {
	path      string
	method    string
	clientUrl string
	timeout   time.Duration
	tag       string
}

func NewDual(cfg DualConfig) *DualRouteBuilder {
	method := cfg.Method
	if method == "" {
		method = "GET"
	}
	return &DualRouteBuilder{
		path:      cfg.Path,
		method:    strings.ToUpper(method),
		clientUrl: cfg.ClientURL,
		timeout:   3 * time.Second,
		tag:       "Unnamed API", // Trans: 未命名接口
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

func (b *DualRouteBuilder) Route(mux Router, handler func(espData string) (any, error)) error {
	return coreHandle(mux, b, handler)
}

// ==========================================
// Generic Builder (Keeps Chaining)
// ==========================================
type TypedDualRouteBuilder[T any] struct {
	*DualRouteBuilder
}

func Typed[T any](cfg DualConfig) *TypedDualRouteBuilder[T] {
	return &TypedDualRouteBuilder[T]{
		DualRouteBuilder: NewDual(cfg),
	}
}

func (b *TypedDualRouteBuilder[T]) Tag(name string) *TypedDualRouteBuilder[T] {
	b.DualRouteBuilder.Tag(name)
	return b
}

func (b *TypedDualRouteBuilder[T]) Timeout(ms int) *TypedDualRouteBuilder[T] {
	b.DualRouteBuilder.Timeout(ms)
	return b
}

func (b *TypedDualRouteBuilder[T]) Handle(mux Router, handler func(espData T) (any, error)) error {
	return coreHandle(mux, b.DualRouteBuilder, func(rawString string) (any, error) {
		var parsedData T
		var zeroValue T

		// 1. String passthrough
		if _, isString := any(zeroValue).(string); isString {
			parsedData = any(rawString).(T)
			return handler(parsedData)
		}

		// 2. JSON parsing
		if err := json.Unmarshal([]byte(rawString), &parsedData); err != nil {
			return map[string]string{
				"error":   "Hardware data parsing failed", // Trans: 硬件数据解析失败
				"details": err.Error(),
				"raw":     rawString,
			}, nil
		}
		return handler(parsedData)
	})
}

// ==========================================
// Core Logic
// ==========================================
func coreHandle(mux Router, b *DualRouteBuilder, handler func(string) (any, error)) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during route registration [%s] %s: %v", b.method, b.path, r) // Trans: 注册路由时发生崩溃
		}
	}()

	fmt.Printf("📌 Registered Route: [%s] %s %s (Timeout: %v)\n", b.tag, b.method, b.path, b.timeout) // Trans: 注册路由

	mux.HandleFunc(b.path, func(w http.ResponseWriter, r *http.Request) {
		// 1. Verify HTTP Method
		if r.Method != b.method {
			http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

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
			http.Error(w, "Failed to build request", http.StatusInternalServerError) // Trans: 构建请求失败
			return
		}

		if ct := r.Header.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}

		client := &http.Client{}
		resp, err := client.Do(req)

		var rawString string
		if err != nil {
			rawString = fmt.Sprintf(`{"error": "Failed to connect to hardware: %v"}`, err) // Trans: 无法连接硬件
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

	return nil
}