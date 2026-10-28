# 🚀 Go-Dual-Router

A lightweight, high-performance **Dual-Route framework** for IoT and Edge Computing, inspired by Ktor & Spring MVC.

（一个受 Ktor 和 Spring 启发的轻量级、高性能物联网双面路由框架。）

## ✨ 特性
- 🪄 **声明式链式调用**：像写 DSL 一样优雅地定义路由。
- 🚦 **内置并发背压**：天然防打挂，保护脆弱的 ESP32 等微控制器。
- 🔄 **参数无缝透传**：自动将前端请求的 Query 和 Body 转发给硬件设备。
- 🧠 **智能响应**：自动识别 HTML 并渲染，或自动将 Struct 序列化为 JSON。

---

## 📦 快速开始 (Quick Start)

### 1. 安装
确保你已经初始化了 Go 模块（`go mod init`），然后运行：
```bash
go get github.com/cosin-coder/go-dual