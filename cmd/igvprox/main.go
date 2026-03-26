package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/compgenlab/igvprox/internal/config"
	"github.com/compgenlab/igvprox/internal/discovery"
	igvserver "github.com/compgenlab/igvprox/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("igvprox: %v", err)
	}
}

func run() error {
	defaultSocketHelp, err := config.ResolveSocketPath("")
	if err != nil {
		return err
	}
	defaultBrowserURL := "http://localhost:5000"

	cfgPath := flag.String("config", "", "config file path")
	genome := flag.String("genome", "", "reference genome id")
	flag.StringVar(genome, "g", "", "reference genome id")
	recursive := flag.Bool("recursive", false, "recursively discover supported files in directory arguments")
	flag.BoolVar(recursive, "R", false, "recursively discover supported files in directory arguments")
	socketHelp := fmt.Sprintf("unix socket path (default %q, or $XDG_RUNTIME_DIR/igvprox.sock when XDG_RUNTIME_DIR is set)", defaultSocketHelp)
	socketPath := flag.String("socket", "", socketHelp)
	flag.StringVar(socketPath, "s", "", socketHelp)
	browserURL := flag.String("open-browser-url", "", fmt.Sprintf("local browser URL hint to print (default %q)", defaultBrowserURL))
	allowMissingIndex := flag.Bool("allow-missing-index", false, "allow indexed formats without a discovered index")
	verbose := flag.Bool("verbose", false, "enable verbose logging")
	flag.BoolVar(verbose, "v", false, "enable verbose logging")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s [flags] <path> [<path> ...]\n\n", os.Args[0])
		fmt.Fprintln(out, "Serve discovered genomics files over a UNIX-socket HTTP proxy for local viewing in igv.js.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Default SSH forwarding:")
		fmt.Fprintf(out, "  ssh -L 5000:%s user@cluster\n", defaultSocketHelp)
		fmt.Fprintf(out, "  then open %s\n\n", defaultBrowserURL)
		fmt.Fprintln(out, "Flags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		return errors.New("at least one file or directory path is required")
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	effectiveGenome := firstNonEmpty(*genome, cfg.Genome, "hg38")
	effectiveBrowserURL := firstNonEmpty(*browserURL, cfg.BrowserURL, defaultBrowserURL)
	effectiveSocketPath, err := config.ResolveSocketPath(firstNonEmpty(*socketPath, cfg.SocketPath, ""))
	if err != nil {
		return err
	}
	effectiveAllowMissingIndex := *allowMissingIndex || cfg.AllowMissingIndex

	opts := discovery.Options{
		Recursive:         *recursive,
		AllowMissingIndex: effectiveAllowMissingIndex,
	}
	paths := flag.Args()
	files, warnings, err := discovery.Collect(paths, opts)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no supported genomics files were found")
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	if !*verbose {
		log.SetOutput(os.Stdout)
		log.SetFlags(0)
	}

	for _, warning := range warnings {
		log.Printf("warning: %s", warning)
	}

	if err := os.Remove(effectiveSocketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing socket: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(effectiveSocketPath), 0o755); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}

	listener, err := net.Listen("unix", effectiveSocketPath)
	if err != nil {
		return fmt.Errorf("listen on unix socket: %w", err)
	}
	if err := os.Chmod(effectiveSocketPath, 0o600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	app := igvserver.New(igvserver.Options{
		Genome:         effectiveGenome,
		BrowserURL:     effectiveBrowserURL,
		Files:          files,
		ConstantTracks: cfg.ConstantTracks,
		Verbose:        *verbose,
	})

	srv := &http.Server{
		Handler:           app.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}

	log.Printf("serving %d tracks", len(files))
	if cfg.Path != "" {
		log.Printf("config: %s", cfg.Path)
	}
	log.Printf("remote socket: %s", effectiveSocketPath)
	log.Printf("local browser URL: %s", effectiveBrowserURL)
	log.Printf("example ssh tunnel:")
	log.Printf("  ssh -L 5000:%s user@cluster", effectiveSocketPath)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(listener)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	if err := os.Remove(effectiveSocketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cleanup socket: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
