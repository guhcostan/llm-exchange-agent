package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"llm-share/agent/internal/config"
	"llm-share/agent/internal/platform"
	"llm-share/agent/internal/runtime"
)

func main() {
	configPath := flag.String("config", envOr("AGENT_CONFIG", "config.yaml"), "path to agent config YAML")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	rt, err := newRuntime(cfg)
	if err != nil {
		log.Fatalf("runtime: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := platform.NewClient(cfg, rt, log.Default())

	for {
		if err := client.Run(ctx); err != nil {
			if ctx.Err() != nil {
				log.Println("agent stopped")
				return
			}
			log.Printf("connection lost: %v; reconnecting in 5s", err)
			select {
			case <-ctx.Done():
				log.Println("agent stopped")
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func newRuntime(cfg config.Config) (runtime.Client, error) {
	switch cfg.Runtime {
	case "ollama":
		return runtime.NewOllama(cfg.RuntimeURL), nil
	case "vllm":
		return runtime.NewVLLM(cfg.RuntimeURL), nil
	default:
		return nil, fmt.Errorf("unsupported runtime %q (expected ollama or vllm)", cfg.Runtime)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
