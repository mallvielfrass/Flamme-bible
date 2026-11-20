package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("команда: login, space, sync, resolve, build, install, plan, bible")
	}
	switch args[0] {
	case "login":
		return cmdLogin(args[1:])
	case "space":
		return cmdSpace(args[1:])
	case "sync":
		return cmdSync(args[1:])
	case "resolve":
		return cmdResolve(args[1:])
	case "build":
		return cmdBuild(args[1:])
	case "install":
		return cmdInstall(args[1:])
	case "plan":
		return cmdPlan(args[1:])
	case "bible":
		return cmdBible(args[1:])
	default:
		return fmt.Errorf("команда: login, space, sync, resolve, build, install, plan, bible")
	}
}

func cmdLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	serverURL := fs.String("server", "", "адрес сервера")
	email := fs.String("email", "", "почта")
	password := fs.String("password", "", "пароль")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *serverURL == "" || *email == "" || *password == "" {
		return fmt.Errorf("нужны --server, --email и --password")
	}
	sess, err := login(*serverURL, *email, *password)
	if err != nil {
		return err
	}
	if err := saveSession(sess); err != nil {
		return err
	}
	fmt.Printf("пространство %s\n", sess.DimensionID)
	return nil
}

func cmdSpace(args []string) error {
	if len(args) != 1 || !validUUID(args[0]) {
		return fmt.Errorf("нужен id пространства")
	}
	sess, err := loadSession()
	if err != nil {
		return err
	}
	sess.DimensionID = args[0]
	if err := saveSession(sess); err != nil {
		return err
	}
	fmt.Printf("пространство %s\n", sess.DimensionID)
	return nil
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	apply := fs.Bool("apply", false, "заменить файл модуля")
	remote := fs.Bool("remote", false, "синхронизировать с сервером")
	dbPath := fs.String("db", "", "файл базы")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || fs.NArg() != 1 {
		return fmt.Errorf("нужны --db и файл модуля")
	}
	store, err := OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	var report string
	if *remote {
		report, err = store.RemoteSync(context.Background(), fs.Arg(0), *apply)
	} else {
		report, err = store.Sync(fs.Arg(0), *apply)
	}
	fmt.Print(report)
	return err
}

func cmdResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	apply := fs.Bool("apply", false, "записать выбранный текст")
	dbPath := fs.String("db", "", "файл базы")
	fbid := fs.String("fbid", "", "песня")
	take := fs.String("take", "", "server или local")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *fbid == "" || fs.NArg() != 1 {
		return fmt.Errorf("нужны --db, --fbid и файл модуля")
	}
	store, err := OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	report, err := store.Resolve(context.Background(), fs.Arg(0), *fbid, *take, *apply)
	fmt.Print(report)
	return err
}

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	apply := fs.Bool("apply", false, "заменить файл модуля")
	dbPath := fs.String("db", "", "файл базы")
	book := fs.String("book", "", "сборник")
	answers := map[string]string{}
	fs.Func("retain", "оставить песню", func(v string) error {
		answers[v] = "retain"
		return nil
	})
	fs.Func("drop", "принять удаление", func(v string) error {
		answers[v] = "drop"
		return nil
	})
	fs.Func("suppress", "скрыть песню родителя", func(v string) error {
		answers[v] = "suppress"
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *book == "" || fs.NArg() != 1 {
		return fmt.Errorf("нужны --db, --book и файл модуля")
	}
	store, err := OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	report, err := store.Build(context.Background(), fs.Arg(0), *book, answers, *apply)
	fmt.Print(report)
	return err
}

func cmdBible(args []string) error {
	fs := flag.NewFlagSet("bible", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	writeID := fs.Bool("write-id", false, "записать проверенный BibleId")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("нужна папка модуля")
	}
	dir := fs.Arg(0)
	if *writeID {
		if err := writeVerifiedBibleID(dir); err != nil {
			return err
		}
	}
	index, err := readBible(dir)
	if err != nil {
		return err
	}
	fmt.Print(formatBible(index))
	return nil
}

func cmdPlan(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("нужен файл плана")
	}
	sync, err := syncPlan(args[0])
	if err != nil {
		return err
	}
	fmt.Print(formatPlan(sync))
	return nil
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "копия с установками VisioBible")
	dir := fs.String("dir", "", "выбранная папка Modules")
	dbPath := fs.String("db", "", "файл базы")
	apply := fs.Bool("apply", false, "заменить файлы выбранной папки")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("нужен --root")
	}
	found, err := findCatalogs(*root)
	if err != nil {
		return err
	}
	chosen, err := selectCatalog(found, *dir)
	if err != nil {
		return err
	}
	fmt.Printf("папка %s\n", chosen)
	fmt.Println("замена модуля делается при закрытой VisioBible")
	if *dbPath == "" {
		fmt.Println("dry-run: install --db путь")
		fmt.Println("применение: install --apply")
		return nil
	}
	files, err := moduleFiles(chosen)
	if err != nil {
		return err
	}
	store, err := OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	for _, path := range files {
		if !insideCatalog(path, chosen) {
			return fmt.Errorf("файл не в выбранной папке")
		}
		report, err := store.Sync(path, *apply)
		fmt.Print(report)
		if err != nil {
			return err
		}
	}
	return nil
}
