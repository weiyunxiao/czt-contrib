package consul

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	serverAddr := "172.17.0.1:8888"

	// 创建一个HTTP路由器
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("Received health check request from %s\n", r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// 创建HTTP服务器
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	// 启动HTTP服务器
	go func() {
		fmt.Printf("Starting HTTP server on %s...\n", serverAddr)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		} else if err == nil {
			fmt.Println("HTTP server stopped normally")
		}
	}()

	// 等待中断信号以优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("Shutting down server...")

	// 设置5秒的超时时间来关闭服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("Error shutting down HTTP server: %v\n", err)
	} else {
		fmt.Println("HTTP server shutdown completed")
	}
}
