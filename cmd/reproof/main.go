// Command reproof 是构建图可复现性隔离证明服务的进程入口。
//
// 支持标志：
//
//	--addr       监听地址（默认 :8090）
//	--db         SQLite 数据库路径（默认 ./reproof.db）
//	--smoke-test 运行端到端自检后退出（不启动 HTTP 服务）
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"task191-reproof/internal/httpapi"
	"task191-reproof/internal/service"
	"task191-reproof/internal/store"
)

func main() {
	var (
		addr      = flag.String("addr", ":8090", "HTTP 监听地址")
		dbPath    = flag.String("db", "reproof.db", "SQLite 数据库路径")
		smoke     = flag.Bool("smoke-test", false, "运行端到端自检后退出")
		keepDB    = flag.Bool("keep-db", false, "自检保留数据库文件（调试）")
		smokePath = flag.String("smoke-db", "", "自检数据库路径（默认临时文件）")
	)
	flag.Parse()

	if *smoke {
		opts := service.SmokeOptions{DBPath: *smokePath, KeepDB: *keepDB}
		if err := service.RunSmoke(opts); err != nil {
			log.Fatalf("smoke-test 失败: %v", err)
		}
		fmt.Println("smoke-test 通过：闭环、持久化恢复、基线比对均符合预期")
		return
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	app := service.New(st)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(app).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("task191-reproof 监听 %s，数据库 %s", *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务错误: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到退出信号，优雅关闭…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭出错: %v", err)
	}
	_ = os.Stdout
}
