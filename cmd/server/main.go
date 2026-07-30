package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
)

func Quit(h *server.Hertz) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	<-quit
	log.Println("正在关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.Shutdown(ctx)
}

func main() {
	h := server.Default(
		server.WithHostPorts(config.Load().Addr),
		server.WithExitWaitTime(5*time.Second),
	)

	go Quit(h)

	h.Spin()
}
