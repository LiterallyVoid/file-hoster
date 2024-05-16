package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

const database_root = "files"
const filename_min_bytes = 2
const filename_bytes = 6

var unsafe_regexp *regexp.Regexp = regexp.MustCompile("[^-a-zA-Z0-9_@.()+]")

//go:embed dist/index.html
var index_html []byte

//go:embed dist/private.html
var private_html string

var private_template = template.Must(template.New("name").Parse(private_html))

//go:embed dist/main.js
var main_js []byte

//go:embed dist/style.css
var style_css []byte

//go:embed dist/favicon.svg
var favicon_svg []byte

//go:embed dist/favicon-180.png
var favicon_180_png []byte

type Metadata struct {
	Filename            string    `json:"filename"`
	UploadDate          time.Time `json:"upload-date"`
	Size                int64     `json:"size"`
	PrivateSelfdestruct bool      `json:"private-selfdestruct,omitempty"`

	Kind string `json:"kind,omitempty"`
}

func uploadFile(filename string, data io.Reader, private_selfdestruct bool) (string, error) {
	if filename != sanitize(filename) {
		panic("filename wasn't sanitized!")
	}

	name := ""
	path := ""
	var file *os.File

	for range 10 {
		bytes := make([]byte, filename_bytes)
		_, err := rand.Read(bytes)
		if err != nil {
			return "", err
		}

		name = base64.RawURLEncoding.EncodeToString(bytes)
		path = database_root + "/" + name

		file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}

		break
	}
	if file == nil {
		return "", errors.New("couldn't create file")
	}

	defer file.Close()

	size, err := io.Copy(file, data)
	if err != nil {
		return "", err
	}

	metadata := Metadata{
		Filename:            filename,
		UploadDate:          time.Now(),
		Size:                size,
		PrivateSelfdestruct: private_selfdestruct,

		Kind: "upload",
	}

	metadata_marshaled, err := json.Marshal(metadata)
	if err != nil {
		panic("Couldn't encode metadata to JSON (should never happen!)")
	}

	err = os.WriteFile(path+".meta", metadata_marshaled, 0644)
	if err != nil {
		fmt.Printf("Couldn't write metadata: %q", err)
		return "", err
	}

	return name, nil
}

type UploadResponseUpload struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
}

type UploadResponse struct {
	Ok      bool                   `json:"ok"`
	Uploads []UploadResponseUpload `json:"uploads"`
}

func sanitize(s string) string {
	return unsafe_regexp.ReplaceAllString(s, "-")
}

type Configuration struct {
	Users []string
}

func authenticated(cfg *Configuration, userid string) bool {
	h := crypto.SHA512.New()
	h.Write([]byte(userid))
	user_secret_hashed := h.Sum(nil)

	for _, id := range cfg.Users {
		h = crypto.SHA512.New()
		h.Write([]byte(id))
		secret_hashed := h.Sum(nil)

		if subtle.ConstantTimeCompare(user_secret_hashed, secret_hashed) == 1 {
			return true
		}
	}

	return false
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("expected subcommand: `serve` or `newuser`")
		os.Exit(1)
	}

	if os.Args[1] == "newuser" {
		var bytes [9]byte
		_, err := io.ReadFull(rand.Reader, bytes[:])
		if err != nil {
			fmt.Printf("err: %v", err)
			return
		}

		id := base64.URLEncoding.EncodeToString(bytes[:])

		fmt.Printf("%q,\n", id)
		fmt.Printf("%s\n", id)

		return
	}

	if os.Args[1] != "serve" {
		if len(os.Args) < 4 {
			fmt.Println("usage: serve <network> <address>")
			os.Exit(1)
		}

		fmt.Println("expected subcommand: `serve` or `newuser`")
		os.Exit(1)
	}

	network := os.Args[2]
	address := os.Args[3]

	cfg := &Configuration{
		Users: []string{
			"gZM9Vl62YYTP",
		},
	}

	handler := &http.ServeMux{}

	handle_file := func(res http.ResponseWriter, req *http.Request) {
		name_unsafe_with_ext := req.PathValue("name")

		name_unsafe, extension, _ := strings.Cut(name_unsafe_with_ext, ".")
		if len(name_unsafe) > (filename_bytes * 4 / 3) {
			http.NotFound(res, req)
			return
		}

		if unsafe_regexp.FindStringIndex(extension) != nil {
			return
		}

		name_b64_bytes, err := base64.RawURLEncoding.DecodeString(name_unsafe)

		if err != nil || len(name_b64_bytes) < filename_min_bytes || len(name_b64_bytes) > filename_bytes {
			http.NotFound(res, req)
			return
		}
		name := base64.RawURLEncoding.EncodeToString(name_b64_bytes)

		filepath := database_root + "/" + name
		metadata_bytes, err := os.ReadFile(filepath + ".meta")
		if err != nil {
			fmt.Printf("not found: %q\n", name)
			http.NotFound(res, req)
			return
		}

		var metadata Metadata
		err = json.Unmarshal(metadata_bytes, &metadata)
		if err != nil {
			fmt.Printf("error loading metadata for %q: %q\n", name, err)
			http.NotFound(res, req)
			return
		}

		if metadata.PrivateSelfdestruct {
			if !authenticated(cfg, req.FormValue("password")) {
				type Private struct {
					Redirect string
				}

				res.Header().Add("Content-Type", "text/html; charset=utf-8")

				res.WriteHeader(http.StatusUnauthorized)
				err := private_template.Execute(
					res,
					&Private{
						Redirect: req.PathValue("name"),
					},
				)
				if err != nil {
					fmt.Printf("err: %v", err)
				}

				return
			}

			err := os.Remove(filepath + ".meta")
			if err != nil {
				fmt.Printf("error self destructing file: %q\n", name)
				http.NotFound(res, req)
				return
			}
		}

		file, err := os.Open(filepath)
		if err != nil {
			fmt.Printf("error loading file for %q: %q\n", name, err)
			http.NotFound(res, req)
			return
		}

		if metadata.PrivateSelfdestruct {
			_ = os.Remove(filepath)
		}

		filename_safe := unsafe_regexp.ReplaceAllString(metadata.Filename, "-")

		res.Header().Add("Content-Disposition", "filename=\""+filename_safe+"\"")

		content_type := mime.TypeByExtension(path.Ext(metadata.Filename))
		res.Header().Add("Content-Type", content_type)

		io.Copy(res, file)
	}
	handler.HandleFunc("GET /{name}", handle_file)
	handler.HandleFunc("POST /{name}", handle_file)

	static := func(path string, data []byte) func(http.ResponseWriter, *http.Request) {
		return func(res http.ResponseWriter, req *http.Request) {
			http.ServeContent(res, req, path, time.Time{}, bytes.NewReader(data))
		}
	}
	handler.HandleFunc("GET /", static("dist/index.html", index_html))
	handler.HandleFunc("GET /main.js", static("dist/main.js", main_js))
	handler.HandleFunc("GET /style.css", static("dist/style.css", style_css))
	handler.HandleFunc("GET /favicon.svg", static("dist/favicon.svg", favicon_svg))
	handler.HandleFunc("GET /favicon-180.png", static("dist/favicon-180.png", favicon_180_png))

	handler.HandleFunc("POST /new", func(res http.ResponseWriter, req *http.Request) {
		part_reader, err := req.MultipartReader()
		if err != nil {
			fmt.Printf("error: %q\n", err)
			res.WriteHeader(http.StatusBadRequest)
			return
		}

		var uploads []UploadResponseUpload

		var is_authenticated bool

		var private_selfdestruct bool

		for {
			part, err := part_reader.NextPart()
			if err == io.EOF {
				break
			}

			if err != nil {
				res.WriteHeader(http.StatusBadRequest)
				return
			}

			field := part.FormName()
			if field == "password" {
				userid_bytes, _ := io.ReadAll(part)
				is_authenticated = authenticated(cfg, string(userid_bytes))

				if is_authenticated {
					continue
				}

				res.WriteHeader(http.StatusUnauthorized)
				return
			}

			if field == "private-selfdestruct" {
				option_value, _ := io.ReadAll(part)
				if string(option_value) == "on" {
					private_selfdestruct = true
				}

				continue
			}

			if field != "files" {
				res.WriteHeader(http.StatusBadRequest)
				return
			}

			if !is_authenticated {
				res.WriteHeader(http.StatusUnauthorized)
				return
			}

			filename := part.FileName()
			filename = sanitize(filename)
			fmt.Printf("Got part with name: %q\n", filename)

			id, err := uploadFile(filename, part, private_selfdestruct)
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				return
			}

			fmt.Printf("Uploaded file: %q\n", id)

			extension := path.Ext(filename)
			extension = sanitize(extension)

			uploads = append(uploads, UploadResponseUpload{
				URL:      "/" + id + extension,
				Filename: filename,
			})
		}

		res.Header().Add("Content-Type", "application/json")

		res.WriteHeader(http.StatusOK)

		response, err := json.Marshal(UploadResponse{
			Ok:      true,
			Uploads: uploads,
		})
		if err != nil {
			panic(err)
		}

		// No idea what to do with an error here..
		_, _ = res.Write(response)
	})

	if network == "unix" {
		os.Remove(address)
	}

	listener, err := net.Listen(network, address)
	if err != nil {
		fmt.Printf("error listening: %v", err)
		os.Exit(1)
	}

	if network == "unix" {
		os.Chmod(address, 0666)
	}

	fmt.Printf("serving: %v\n", address)
	http.Serve(listener, handler)
}
