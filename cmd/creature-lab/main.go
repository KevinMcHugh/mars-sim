// Command creature-lab serves Creature Lab: the alien species catalog and
// its sprites, over a web UI, a JSON API and MCP. See docs/creature-lab.md.
//
//	creature-lab [serve]                    run the server (DATABASE_URL, PORT, PUBLIC_URL)
//	creature-lab keys create --name NAME    mint an api key (printed once)
//	creature-lab keys list                  list live api keys
//	creature-lab keys rotate --id ID        replace a key, keeping its name
//	creature-lab keys delete --id ID        revoke a key and every token it approved
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/xid"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/web"
)

func main() {
	var err error
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "", "serve":
		err = serve()
	case "keys":
		err = keys(args)
	default:
		err = fmt.Errorf("unknown command %q; use serve or keys", cmd)
	}
	if err != nil {
		slog.Error("creature-lab", "err", err)
		os.Exit(1)
	}
}

func serve() error {
	ctx := context.Background()
	pool, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	issuer := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	port := envOr("PORT", "8080")
	if issuer == "" {
		issuer = "http://localhost:" + port
	}
	srv := web.New(creaturelab.NewService(pool, issuer), issuer)
	slog.Info("creature-lab listening", "port", port, "public_url", issuer, "generator", creaturelab.GeneratorRev())
	hs := &http.Server{Addr: ":" + port, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	return hs.ListenAndServe()
}

func keys(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: keys <create|list|rotate|delete> ...")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("keys "+sub, flag.ExitOnError)
	name := fs.String("name", "", "label for the key (create)")
	id := fs.String("id", "", "key id (rotate, delete)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := db.New(pool)

	switch sub {
	case "create":
		if *name == "" {
			return errors.New("--name is required")
		}
		return mint(ctx, q, *name)
	case "list":
		rows, err := q.ListAPIKeys(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tCREATED\tLAST USED")
		for _, r := range rows {
			last := "-"
			if r.LastUsedAt.Valid {
				last = r.LastUsedAt.Time.UTC().Format(time.DateTime)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.Name, r.CreatedAt.Time.UTC().Format(time.DateTime), last)
		}
		return tw.Flush()
	case "rotate", "delete":
		if *id == "" {
			return errors.New("--id is required")
		}
		old, err := q.GetAPIKey(ctx, *id)
		if err != nil {
			return fmt.Errorf("look up key %s: %w", *id, err)
		}
		// Deleting the key also cuts off its OAuth tokens and web sessions:
		// their lookups join on a live key.
		if err := q.DeleteAPIKey(ctx, old.ID); err != nil {
			return err
		}
		if sub == "delete" {
			fmt.Printf("deleted key %s (%s)\n", old.ID, old.Name)
			return nil
		}
		fmt.Printf("rotated key %s (%s)\n", old.ID, old.Name)
		return mint(ctx, q, old.Name)
	default:
		return fmt.Errorf("unknown keys subcommand %q", sub)
	}
}

func mint(ctx context.Context, q *db.Queries, name string) error {
	raw, err := auth.GenerateKey()
	if err != nil {
		return err
	}
	row, err := q.CreateAPIKey(ctx, db.CreateAPIKeyParams{ID: xid.New().String(), Name: name, KeyHash: auth.Hash(raw)})
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	fmt.Printf("key_id:  %s\napi_key: %s\n\nSave the key now; it is never shown again.\n", row.ID, raw)
	return nil
}

func openPool(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	return pool, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
