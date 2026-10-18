// Command pdx-server ist der Render-Dienst von pdx über HTTP.
//
//	pdx-server -config pdx-server.json
//	pdx-server hash-key < schluessel.txt
//
// Die Konfiguration ist JSON (siehe server.Config), API-Schlüssel stehen
// darin nur als SHA-256-Hash. hash-key liest den Schlüssel von der
// Standardeingabe, damit er nicht in der Prozessliste oder Shell-Historie
// landet.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/server"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "hash-key" {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		key := strings.TrimRight(line, "\r\n")
		if (err != nil && err != io.EOF) || len(key) < 16 {
			fmt.Fprintln(stderr, "pdx-server hash-key: Schlüssel mit mindestens 16 Zeichen auf der Standardeingabe erwartet")
			return 2
		}
		fmt.Fprintln(stdout, server.HashKey(key))
		return 0
	}
	fs := flag.NewFlagSet("pdx-server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", os.Getenv("PDX_SERVER_CONFIG"), "Konfigurationsdatei (JSON), Vorgabe aus PDX_SERVER_CONFIG")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *cfgPath == "" {
		fmt.Fprintln(stderr, "pdx-server: -config oder PDX_SERVER_CONFIG fehlt")
		return 2
	}
	cfg, err := server.LoadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "pdx-server: %v\n", err)
		return 2
	}
	if cfg.TemplateDir == "" {
		fmt.Fprintln(stderr, "pdx-server: templateDir fehlt in der Konfiguration")
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stdout, nil))
	s, err := server.New(cfg, &server.DirStore{Dir: cfg.TemplateDir}, log)
	if err != nil {
		fmt.Fprintf(stderr, "pdx-server: %v\n", err)
		return 2
	}
	hs := &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.Timeout.Duration + 10*time.Second,
		WriteTimeout:      cfg.Timeout.Duration + 10*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("gestartet", "adresse", cfg.Addr, "version", pdx.Version, "schluessel", len(cfg.Keys))
	select {
	case err := <-errc:
		log.Error("beendet", "fehler", err.Error())
		return 1
	case <-ctx.Done():
	}
	// Laufende Anfragen dürfen zu Ende laufen.
	sctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout.Duration+5*time.Second)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("Herunterfahren", "fehler", err.Error())
		return 1
	}
	log.Info("beendet")
	return 0
}
