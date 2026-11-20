package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	server := fs.String("server", envOr("FLAMMEBIBLE_SERVER", "http://127.0.0.1:8080"), "адрес API")
	token := fs.String("token", os.Getenv("FLAMMEBIBLE_TOKEN"), "токен администратора")
	version := fs.String("version", "", "версия")
	linux := fs.String("linux", "", "файл linux")
	windows := fs.String("windows", "", "файл windows")
	macos := fs.String("macos", "", "файл macos")
	notes := fs.String("notes", "", "заметка к релизу")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	files := map[string]string{}
	if *linux != "" {
		files["linux_file"] = *linux
	}
	if *windows != "" {
		files["windows_file"] = *windows
	}
	if *macos != "" {
		files["macos_file"] = *macos
	}
	if strings.TrimSpace(*version) == "" || strings.TrimSpace(*token) == "" || len(files) == 0 {
		fmt.Fprintln(os.Stderr, "нужны -version, -token и хотя бы один из -linux, -windows, -macos")
		return 2
	}
	if err := upload(strings.TrimRight(*server, "/"), *token, strings.TrimSpace(*version), *notes, files); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func upload(server, token, version, notes string, files map[string]string) error {
	body, size, contentType, cleanup, err := buildBody(version, notes, files)
	if err != nil {
		return err
	}
	defer cleanup()
	req, err := http.NewRequest(http.MethodPost, server+"/api/admin/releases", body)
	if err != nil {
		return errors.New("ошибка запроса")
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка запроса: %v", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("ошибка запроса: %v", err)
	}
	if res.StatusCode/100 == 2 {
		fmt.Printf("версия %s\n", version)
		return nil
	}
	var parsed struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &parsed)
	if res.StatusCode == http.StatusConflict {
		return fmt.Errorf("версия %s уже выпущена, укажите новую через -version\nHTTP %d %s", version, res.StatusCode, parsed.Error)
	}
	if parsed.Error != "" {
		return fmt.Errorf("HTTP %d %s", res.StatusCode, parsed.Error)
	}
	return fmt.Errorf("HTTP %d", res.StatusCode)
}

func buildBody(version, notes string, files map[string]string) (*os.File, int64, string, func(), error) {
	fail := func() (*os.File, int64, string, func(), error) {
		return nil, 0, "", func() {}, errors.New("не удалось подготовить загрузку")
	}
	file, err := os.CreateTemp("", "flammebible-upload-*")
	if err != nil {
		return fail()
	}
	done := false
	cleanup := func() {
		file.Close()
		os.Remove(file.Name())
	}
	defer func() {
		if !done {
			cleanup()
		}
	}()
	writer := multipart.NewWriter(file)
	if err := writer.WriteField("version", version); err != nil {
		return fail()
	}
	if notes != "" {
		if err := writer.WriteField("notes", notes); err != nil {
			return fail()
		}
	}
	names := []string{"linux_file", "windows_file", "macos_file"}
	for _, name := range names {
		path := files[name]
		if path == "" {
			continue
		}
		if err := copyFile(writer, name, path); err != nil {
			return nil, 0, "", func() {}, err
		}
	}
	if err := writer.Close(); err != nil {
		return fail()
	}
	size, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fail()
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail()
	}
	done = true
	return file, size, writer.FormDataContentType(), cleanup, nil
}

func copyFile(writer *multipart.Writer, field, path string) error {
	src, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("нет файла %s", path)
		}
		return fmt.Errorf("недоступен файл %s", path)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil || info.IsDir() {
		return fmt.Errorf("нет файла %s", path)
	}
	part, err := writer.CreateFormFile(field, "flammebible")
	if err != nil {
		return errors.New("не удалось подготовить загрузку")
	}
	if _, err := io.Copy(part, src); err != nil {
		return errors.New("не удалось подготовить загрузку")
	}
	return nil
}
