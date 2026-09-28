package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aklkbqx/agy-swap/internal/demoserver"
)

func main() {
	origin := os.Getenv("AGY_DEMO_ORIGIN")
	if origin == "" {
		log.Fatal("AGY_DEMO_ORIGIN is required")
	}
	binary := os.Getenv("AGY_DEMO_BINARY")
	if binary == "" {
		binary = "/usr/local/bin/agy-swap-demo"
	}
	server := &http.Server{
		Addr: ":8080", ReadHeaderTimeout: 5 * time.Second,
		Handler: demoserver.New(demoserver.Config{Origin: origin, Binary: binary, TempRoot: os.Getenv("AGY_DEMO_TMP")}),
	}
	log.Fatal(server.ListenAndServe())
}
