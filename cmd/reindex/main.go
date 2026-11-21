package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"flammebible/internal/server"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("reindex", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("import", "", "каталог BibleQuote: bibleqt.ini и файлы книг")
	moduleID := fs.String("module", "", "id модуля, в который записать стихи")
	process := fs.Bool("process", false, "сразу обработать очередь, а не только поставить задания")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if (*dir == "") != (*moduleID == "") {
		fmt.Fprintln(os.Stderr, "флаги -import и -module задаются вместе")
		return 2
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "нужен DATABASE_URL")
		return 2
	}
	embedURL := strings.TrimSpace(os.Getenv("EMBED_URL"))
	if embedURL == "" {
		fmt.Fprintln(os.Stderr, "нужен EMBED_URL")
		return 2
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	if err := server.Migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	api := server.New(pool)
	api.SetEmbedder(server.NewHTTPEmbedder(embedURL, os.Getenv("EMBED_MODEL")))
	api.SetEmbedConfig(server.EmbedConfigFromEnv())
	if err := api.EnsureEmbedSettings(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *dir != "" {
		n, err := api.ImportBibleModule(ctx, *moduleID, *dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("стихи модуля %s: %d\n", *moduleID, n)
	}
	stats, err := api.ReindexAll(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("части песен в очередь: %d, без изменений %d\n", stats.SongsEmbedded, stats.SongsSkipped)
	fmt.Printf("модули: %d, отрывки в очередь: %d, без изменений %d\n", stats.Modules, stats.PassagesEmbedded, stats.PassagesSkipped)
	if !*process {
		return 0
	}
	if err := api.DrainEmbedQueue(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("очередь обработана")
	return 0
}
