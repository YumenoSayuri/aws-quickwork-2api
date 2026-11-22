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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"quickwork-openai/internal/config"
	"quickwork-openai/internal/store"
	"quickwork-openai/internal/wui"
)

// console is the management console wired to one config and store.
type console struct {
	srv  *http.Server
	addr string
}

// startConsole builds the console described by cfg.
func startConsole(cfg *config.Config, st *store.Store, configPath string) (*console, error) {
	if !cfg.WUI.Enabled {
		return nil, errors.New("wui.enabled is false")
	}
	if strings.TrimSpace(cfg.WUI.Password) == "" {
		return nil, errors.New("wui.password is empty")
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	workDir := filepath.Dir(cfg.Path)
	handler := wui.New(cfg, st, wui.Options{
		ConfigPath: configPath,
		Binary:     binary,
		WorkDir:    workDir,
		Addr:       cfg.WUI.Addr,
		Password:   cfg.WUI.Password,
		SessionTTL: cfg.WUI.Session.Duration(),
	})
	return &console{
		srv: &http.Server{
			Addr:              cfg.WUI.Addr,
			Handler:           handler.Handler(),
			ReadHeaderTimeout: 15 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		addr: cfg.WUI.Addr,
	}, nil
}

// run serves the console until stop is closed.
func (c *console) run(stop context.Context) {
	listenErr := make(chan error, 1)
	go func() {
		log.Printf("management console listening on http://%s/", c.addr)
		if err := c.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- err
		}
	}()
	select {
	case <-stop.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.srv.Shutdown(shutdown)
	case err := <-listenErr:
		log.Printf("console listen: %v", err)
	}
}

func webui(args []string) int {
	fs := flag.NewFlagSet("webui", flag.ExitOnError)
	configPath := fs.String("config", "config.json", "path to config.json")
	_ = fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	if !cfg.WUI.Enabled {
		cfg.WUI.Enabled = true
	}
	st, err := openStore(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()

	con, err := startConsole(cfg, st, *configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "console:", err)
		return 1
	}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	con.run(stop)
	return 0
}
