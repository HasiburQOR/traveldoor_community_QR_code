// Command app runs the QR Social Profile Platform HTTP server.
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

	"traveldoor/qrprofile/internal/auth"
	"traveldoor/qrprofile/internal/config"
	"traveldoor/qrprofile/internal/handlers"
	"traveldoor/qrprofile/internal/seed"
	"traveldoor/qrprofile/internal/store"
)

func main() {
	var (
		seedFile     = flag.String("seed", "", "path to a JSON seed file to import, then exit")
		doSeed       = flag.Bool("seed-default", false, "import the bundled Travel Door Georgia profile, then exit")
		setPassEmail = flag.String("set-password", "", "admin email whose password to reset, then exit")
		setPassValue = flag.String("password", "", "new password to use with -set-password (read from stdin when empty)")
	)
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := config.LoadDotEnv(".env"); err != nil {
		log.Error("load .env", "error", err)
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration", "error", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Error("open database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := bootstrapAdmin(cfg, st, log); err != nil {
		log.Error("bootstrap admin", "error", err)
		os.Exit(1)
	}

	// SEED_DEFAULT=true seeds the bundled profile only when it is missing, so
	// a restart never discards links edited in the admin. SEED_DEFAULT=force
	// re-imports every start, replacing the link set with the bundled one.
	if cfg.SeedDefault {
		var err error
		if cfg.SeedForce {
			err = seed.ImportDefault(st, cfg.UploadDir, log)
		} else {
			err = seed.ImportDefaultIfMissing(st, cfg.UploadDir, log)
		}
		if err != nil {
			log.Error("seed", "error", err)
			os.Exit(1)
		}
	}

	if *setPassEmail != "" {
		if err := setPassword(st, *setPassEmail, *setPassValue, log); err != nil {
			log.Error("set password", "error", err)
			os.Exit(1)
		}
		return
	}

	switch {
	case *doSeed:
		if err := seed.ImportDefault(st, cfg.UploadDir, log); err != nil {
			log.Error("seed", "error", err)
			os.Exit(1)
		}
		return
	case *seedFile != "":
		if err := seed.ImportFile(st, *seedFile, cfg.UploadDir, log); err != nil {
			log.Error("seed", "error", err)
			os.Exit(1)
		}
		return
	}

	srv, err := handlers.New(cfg, st, log)
	if err != nil {
		log.Error("build server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Housekeeping: drop expired sessions periodically.
	stopJanitor := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := st.DeleteExpiredSessions(); err != nil {
					log.Warn("session cleanup", "error", err)
				}
			case <-stopJanitor:
				return
			}
		}
	}()

	go func() {
		log.Info("listening", "addr", cfg.Addr, "base_url", cfg.BaseURL, "env", cfg.AppEnv)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	close(stopJanitor)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("shutdown", "error", err)
	}
	log.Info("stopped")
}

// bootstrapAdmin creates the first admin account from the environment when no
// user exists yet.
func bootstrapAdmin(cfg *config.Config, st *store.Store, log *slog.Logger) error {
	n, err := st.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.AdminBootstrapEmail == "" || cfg.AdminBootstrapPass == "" {
		log.Warn("no admin user exists; set ADMIN_BOOTSTRAP_EMAIL and ADMIN_BOOTSTRAP_PASSWORD to create one")
		return nil
	}
	hash, err := auth.HashPassword(cfg.AdminBootstrapPass)
	if err != nil {
		return fmt.Errorf("bootstrap password: %w", err)
	}
	if _, err := st.CreateUser(cfg.AdminBootstrapEmail, hash); err != nil {
		return err
	}
	log.Info("bootstrap admin created", "email", cfg.AdminBootstrapEmail)
	return nil
}

// setPassword resets an existing admin password. The value comes from the
// -password flag, or from stdin so it never lands in the shell history.
func setPassword(st *store.Store, email, plain string, log *slog.Logger) error {
	if plain == "" {
		fmt.Fprint(os.Stderr, "New password: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		plain = strings.TrimSpace(line)
	}
	hash, err := auth.HashPassword(plain)
	if err != nil {
		return err
	}
	user, err := st.UserByEmail(email)
	if err != nil {
		return fmt.Errorf("no admin account for %q", email)
	}
	if err := st.UpdateUserPassword(user.ID, hash); err != nil {
		return err
	}
	log.Info("password updated", "email", user.Email)
	return nil
}
