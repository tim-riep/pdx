// Command pdx-server is the rendering service of pdx over HTTP.
//
//	pdx-server -config pdx-server.json
//	pdx-server hash-key < schluessel.txt
//	pdx-server licenses
//
// The configuration is JSON (see server.Config), API keys appear in it
// only as SHA-256 hashes. hash-key reads the key from standard input so
// that it does not end up in the process list or the shell history.
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
	"github.com/tim-riep/pdx/sign"
	"github.com/tim-riep/pdx/verapdf"
)

// configure enables veraPDF and signing according to the configuration.
// Password and token come only from the environment, never from a file or
// an argument, and are removed from the environment after loading.
func configure(s *server.Server, cfg *server.Config) error {
	switch v := cfg.VeraPDF; v {
	case "":
	case "docker":
		s.Verifier = verapdf.Docker{Image: verapdf.DockerImage, Platform: verapdf.DockerPlatform}
	case "local":
		r, err := verapdf.Detect(context.Background(), nil)
		if err != nil {
			return err
		}
		s.Verifier = r
	default:
		s.Verifier = verapdf.Local{Path: v}
	}
	if cfg.Signer == "" {
		return nil
	}
	kind, loc, _ := strings.Cut(cfg.Signer, ":")
	switch kind {
	case "pkcs12":
		data, err := os.ReadFile(loc)
		if err != nil {
			return err
		}
		pw, ok := os.LookupEnv("PDX_SIGN_PASSWORD")
		if !ok {
			return errors.New("PDX_SIGN_PASSWORD fehlt für den Signer")
		}
		os.Unsetenv("PDX_SIGN_PASSWORD")
		if s.Signer, err = sign.LoadPKCS12(data, pw); err != nil {
			return err
		}
	case "remote":
		tok := os.Getenv("PDX_SIGN_TOKEN")
		os.Unsetenv("PDX_SIGN_TOKEN")
		rs, err := sign.NewRemoteSigner(context.Background(), loc, tok, nil)
		if err != nil {
			return err
		}
		s.Signer = rs
	default:
		return fmt.Errorf("Signer %q: erlaubt sind pkcs12:DATEI und remote:URL", cfg.Signer)
	}
	if cfg.TSA != "" {
		s.TSA = &sign.TSA{URL: cfg.TSA}
	}
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "licenses" {
		fmt.Fprint(stdout, pdx.License, "\n\n", pdx.ThirdPartyNotices)
		return 0
	}
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
	if err := configure(s, cfg); err != nil {
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
	// Running requests may run to completion.
	sctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout.Duration+5*time.Second)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("Herunterfahren", "fehler", err.Error())
		return 1
	}
	log.Info("beendet")
	return 0
}
