package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"Decision402/internal/agent"
	"Decision402/internal/probe"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "local UI + API + demo sellers")
	web := flag.String("web", "web/dist", "built Vue frontend")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || host != "127.0.0.1" {
		log.Fatal("bind to 127.0.0.1 only")
	}
	cfg, err := probe.LoadConfig(".env")
	if err != nil {
		log.Fatal(err)
	}
	// The UI encrypts personal API keys. The optional shared key comes only
	// from the process environment, not the plaintext .env file.
	cfg.DeepSeekKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if cfg.DeepSeekModel == "" {
		cfg.DeepSeekModel = "deepseek-flash"
	}
	if cfg.DeepSeekURL != "https://api.deepseek.com" {
		log.Fatal("DEEPSEEK_BASE_URL must be https://api.deepseek.com")
	}
	dir := "artifacts/tasks"
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Fatal(err)
	}
	// OS lock is released on process exit. Prevent two instances spending against
	// the same journal/wallet with separate in-memory task states.
	lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Fatal("another Decision402 instance owns the task journal")
	}
	app, err := agent.NewApp(*listen, dir, agent.NewModel(cfg.DeepSeekKey, cfg.DeepSeekModel, cfg.DeepSeekURL), probe.NewClient(cfg.APIKey, 15*time.Second, cfg.LowRiskTraits...), cfg.NormalAddress, cfg.RiskAddress, cfg.LowRiskAddress)
	if err != nil {
		log.Fatal(err)
	}
	handler := app.Handler(*web)
	log.Printf("Decision402: http://%s | model=%s | Base Sepolia only", *listen, cfg.DeepSeekModel)
	log.Fatal((&http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
