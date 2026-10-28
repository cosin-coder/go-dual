package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cosin-coder/go-dual"
)

// 1. 定义硬件设备的数据结构（强类型，自动映射 JSON）
type EspStatus struct {
	Wifi   string `json:"wifi"`
	Ip     string `json:"ip"`
	Rssi   int    `json:"rssi"`
	Uptime int    `json:"uptime"`
	Heap   int    `json:"heap"`
}

func main() {
	// 假设你的 ESP32 IP 为 http://192.168.3.94

	// 2. 注册一个强类型 JSON 接口
	dual.NewDual("/api/v1/status", "GET", "http://192.168.3.94").
		Tag("设备状态").         // 打上日志标签
		Timeout(2000).          // 设置 2 秒超时（底层自动触发 HTTP Timeout）
		GetTyped(func(espData EspStatus) (any, error) {
			// 🌟 魔法发生在这里：
			// 框架自动请求了 ESP32，自动将 JSON 反序列化为 EspStatus 对象！
			fmt.Printf("✅ 信号强度: %d dBm, 剩余内存: %d\n", espData.Rssi, espData.Heap)
			
			// 直接返回 Map，框架会自动将其转为 JSON 格式给前端
			return map[string]any{
				"msg":    "ok",
				"device": espData,
			}, nil
		})

	// 3. 注册一个网页透传接口（支持原样返回 HTML）
	dual.NewDual("/api/v1/home", "GET", "http://192.168.3.94").
		Tag("网页透传").
		Get(func(espHtml string) (any, error) {
			// 只要返回的字符串以 "<" 开头，框架会自动设置 Content-Type: text/html
			// 前端访问 http://localhost:8080/api/v1/home 就能直接看到 ESP32 的控制面板
			return espHtml, nil
		})

	// 4. 注册一个 POST 控制接口（自动透传前端参数给 ESP32）
	dual.NewDual("/api/v1/led", "POST", "http://192.168.3.94").
		Tag("LED控制").
		GetTyped(func(resp string) (any, error) {
			// 注意：前端发来的 Body 和 Query 已被自动透传给 ESP32
			return map[string]any{"msg": "指令已下发", "esp_response": resp}, nil
		})

	// 5. 启动网关服务器
	fmt.Println("🚀 TableBot Gateway running on :8080")
	http.ListenAndServe(":8080", nil)
}