package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aantich/lastapp/internal/control"
)

// fatal logs a startup/command failure and exits. Command errors are
// operator-facing configuration or connection failures; request errors are
// logged by the server with type-only diagnostics.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// configureLogging installs the process-wide structured logger: readable text
// in development, JSON lines otherwise.
func configureLogging(development bool) {
	var h slog.Handler = slog.NewJSONHandler(os.Stderr, nil)
	if development {
		h = slog.NewTextHandler(os.Stderr, nil)
	}
	slog.SetDefault(slog.New(h))
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: lastapp serve|migrate|bootstrap|seed-development|publish-package|retire-package|prune-package-artifacts -config path | runtime-grants -database name -host host")
	}
	command := os.Args[1]
	if command == "runtime-grants" {
		runtimeGrants(os.Args[2:])
		return
	}
	flags := flag.NewFlagSet(command, flag.ExitOnError)
	config := flags.String("config", ".local/config.json", "server configuration file")
	issuer := flags.String("issuer", "", "bootstrap identity issuer")
	subject := flags.String("subject", "", "bootstrap immutable identity subject")
	name := flags.String("name", "Platform operator", "bootstrap name")
	email := flags.String("email", "", "bootstrap email")
	provisionerFile := flags.String("provisioner-file", "", "local development provisioning credential file")
	passwordFile := flags.String("password-file", "", "initial password file for a local bootstrap account")
	manifest := flags.String("manifest", "", "package manifest JSON")
	build := flags.String("build", "", "compiled package App directory")
	source := flags.String("source", "", "matching source ZIP")
	key := flags.String("package", "", "catalog package key to retire")
	version := flags.String("version", "", "catalog version to retire")
	entry := flags.String("entry", "index.html", "compiled App entry point")
	_ = flags.Parse(os.Args[2:]) // ExitOnError: Parse exits on failure
	c, e := control.LoadConfig(*config)
	if e != nil {
		fatal("configuration failed", "error", e)
	}
	configureLogging(c.Development)
	s, e := control.Open(c)
	if e != nil {
		fatal("startup failed", "error", e)
	}
	defer s.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch command {
	case "retire-package":
		if e = control.CheckSchema(ctx, s.DB); e == nil {
			e = s.RetirePackage(ctx, *key, *version)
		}
	case "prune-package-artifacts":
		if e = control.CheckSchema(ctx, s.DB); e == nil {
			var removed int
			removed, e = s.PrunePackageArtifacts(ctx)
			if e == nil {
				fmt.Printf("Removed %d abandoned artifacts\n", removed)
			}
		}
	case "publish-package":
		if e = control.CheckSchema(ctx, s.DB); e == nil {
			e = publishPackage(ctx, s, *manifest, *build, *source, *entry)
		}
	case "migrate":
		e = control.Migrate(ctx, s.DB)
	case "bootstrap":
		if *passwordFile != "" || (*issuer == "" && *subject == "" && *email != "") {
			if *issuer != "" || *subject != "" {
				fatal("choose local account or OIDC bootstrap, not both")
			}
			var secret []byte
			if *passwordFile != "" {
				secret, e = os.ReadFile(*passwordFile)
				if e != nil {
					break
				}
			}
			var uid string
			uid, e = s.BootstrapLocal(ctx, *name, *email, strings.TrimRight(string(secret), "\r\n"))
			if e == nil {
				fmt.Println("Superadmin identity:", uid)
			}
			break
		}
		if *issuer == "" || *subject == "" {
			fatal("issuer and subject are required")
		}
		var uid string
		uid, e = s.Bootstrap(ctx, *issuer, *subject, *name, *email)
		if e == nil {
			fmt.Println("Superadmin identity:", uid)
		}
	case "seed-development":
		var secret []byte
		if *provisionerFile != "" {
			secret, e = os.ReadFile(*provisionerFile)
			if e != nil {
				break
			}
		}
		e = s.SeedDevelopment(ctx, strings.TrimSpace(string(secret)))
	case "serve":
		if e = control.CheckSchema(ctx, s.DB); e != nil {
			break
		}
		if e = s.InitAuth(ctx); e != nil {
			break
		}
		go s.RunWorker(ctx)
		server := &http.Server{Addr: c.Address, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if e := server.Shutdown(shutdown); e != nil {
				slog.Warn("shutdown incomplete", "error", e)
			}
		}()
		slog.Info("Lastapp listening", "console", c.PublicURL+"/console/", "address", c.Address)
		e = server.ListenAndServe()
		if errors.Is(e, http.ErrServerClosed) {
			<-stopped
			e = nil
		}
	default:
		e = fmt.Errorf("unknown command %q", command)
	}
	if e != nil {
		fatal(command+" failed", "error", e)
	}
}

// runtimeGrants prints, one per line, the UPDATE,DELETE grants the runtime
// account needs on the registry. It reads only the embedded migrations, so
// local tooling can apply them without configuration or a database connection.
func runtimeGrants(args []string) {
	flags := flag.NewFlagSet("runtime-grants", flag.ExitOnError)
	database := flags.String("database", "", "registry database name")
	host := flags.String("host", "", "runtime account host")
	_ = flags.Parse(args) // ExitOnError: Parse exits on failure
	statements, e := control.RuntimeGrantStatements(*database, *host)
	if e != nil {
		fatal("runtime-grants failed", "error", e)
	}
	for _, statement := range statements {
		fmt.Println(statement + ";")
	}
}
