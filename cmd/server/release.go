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

func runRelease(args []string) int {
	if len(args) == 0 {
		printReleaseUsage()
		return 2
	}
	switch args[0] {
	case "add":
		return runReleaseAdd(args[1:])
	case "help", "-h", "--help":
		printReleaseUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда release %s\n", args[0])
		printReleaseUsage()
		return 2
	}
}

func printReleaseUsage() {
	fmt.Fprintln(os.Stderr, "использование: flammebible-server release add --version ВЕРСИЯ --linux ФАЙЛ --windows ФАЙЛ [--macos ФАЙЛ] [--notes ТЕКСТ] [--publish]")
	fmt.Fprintln(os.Stderr, "команда пишет файлы в RELEASES_DIR и запись в базу; токен не нужен")
}

type releaseAddFlags struct {
	version    string
	notes      string
	linux      string
	windows    string
	macos      string
	linuxSHA   string
	windowsSHA string
	macosSHA   string
	publish    bool
}

func parseReleaseAdd(args []string) (releaseAddFlags, error) {
	var flags releaseAddFlags
	fs := flag.NewFlagSet("release add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&flags.version, "version", "", "версия")
	fs.StringVar(&flags.notes, "notes", "", "заметка к релизу")
	fs.StringVar(&flags.linux, "linux", "", "файл linux")
	fs.StringVar(&flags.windows, "windows", "", "файл windows")
	fs.StringVar(&flags.macos, "macos", "", "файл macos")
	fs.StringVar(&flags.linuxSHA, "linux-sha256", "", "ожидаемый SHA-256 файла linux")
	fs.StringVar(&flags.windowsSHA, "windows-sha256", "", "ожидаемый SHA-256 файла windows")
	fs.StringVar(&flags.macosSHA, "macos-sha256", "", "ожидаемый SHA-256 файла macos")
	fs.BoolVar(&flags.publish, "publish", false, "сразу опубликовать версию")
	if err := fs.Parse(args); err != nil {
		return releaseAddFlags{}, err
	}
	flags.version = strings.TrimSpace(flags.version)
	flags.notes = strings.TrimSpace(flags.notes)
	if flags.version == "" || (flags.linux == "" && flags.windows == "" && flags.macos == "") {
		return releaseAddFlags{}, errors.New("нужны --version и хотя бы один из --linux, --windows, --macos")
	}
	return flags, nil
}

func runReleaseAdd(args []string) int {
	flags, err := parseReleaseAdd(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "нужен DATABASE_URL")
		return 2
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	files := map[string]string{}
	expect := map[string]string{}
	if flags.linux != "" {
		files["linux"] = flags.linux
		expect["linux"] = flags.linuxSHA
	}
	if flags.windows != "" {
		files["windows"] = flags.windows
		expect["windows"] = flags.windowsSHA
	}
	if flags.macos != "" {
		files["macos"] = flags.macos
		expect["macos"] = flags.macosSHA
	}
	rel, created, err := server.StoreReleaseFiles(ctx, pool, "", flags.version, flags.notes, files, expect)
	if err != nil {
		fmt.Fprintln(os.Stderr, server.ReleaseOpError(err))
		return 1
	}
	if flags.publish {
		rel, err = server.PublishRelease(ctx, pool, "", flags.version)
		if err != nil {
			fmt.Fprintln(os.Stderr, server.ReleaseOpError(err))
			return 1
		}
		fmt.Printf("версия %s опубликована\n", rel.Version)
		return 0
	}
	state := "обновлён черновик"
	if created {
		state = "создан черновик"
	}
	fmt.Printf("версия %s, %s\n", rel.Version, state)
	return 0
}
