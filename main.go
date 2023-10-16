package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"

	"github.com/alecthomas/chroma/quick"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/panic/", panicDemo)
	mux.HandleFunc("/panic-after/", panicAfterDemo)
	mux.HandleFunc("/", hello)
	mux.HandleFunc("/debug/", sourceCodeHandler)
	log.Fatal(http.ListenAndServe(":3000", recoverMw(mux, true)))
}

func sourceCodeHandler(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")
	// testpath := "D:\\Gophercises\\recovery\\main.go"
	file, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b := bytes.NewBuffer(nil)
	_, err = io.Copy(b, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = quick.Highlight(w, b.String(), "go", "html", "monokai")
}

func recoverMw(app http.Handler, dev bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Println(err)
				stack := debug.Stack()
				log.Println(string(stack))
				if !dev {
					http.Error(w, "Something went wrong :(", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "<h1>panic: %v</h1><pre>%s</pre>", err, makeLinks(string(stack)))

			}
		}()
		nrw := &responseWriter{ResponseWriter: w}
		app.ServeHTTP(nrw, r)
		nrw.flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("the responseWriter does not support the Hijacker interface")
	}
	return hijacker.Hijack()
}

func (rw *responseWriter) Flush() {
	flusher, ok := rw.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
		return
	}
}

// type ResponseWriter interface {
// 	Header() Header
// 	Write([]byte) (int, error)
// 	WriteHeader(statusCode int)
// }

type responseWriter struct {
	http.ResponseWriter
	writes [][]byte
	status int
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.writes = append(rw.writes, b)
	return len(b), nil
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
}

func (rw *responseWriter) flush() error {
	if rw.status != 0 {
		rw.ResponseWriter.WriteHeader(rw.status)
	}
	for _, write := range rw.writes {
		_, err := rw.ResponseWriter.Write(write)
		if err != nil {
			return err
		}
	}
	return nil
}

func panicDemo(w http.ResponseWriter, r *http.Request) {
	funcThatPanics()
}

func panicAfterDemo(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "<h1>Hello!</h1>")
	funcThatPanics()
}

func funcThatPanics() {
	panic("Oh no!")
}

func hello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "<h1>Hello!</h1>")
}
// goroutine 12 [running]:
// runtime/debug.Stack()
// 	C:/Program Files/Go/src/runtime/debug/stack.go:24 +0x65
// main.recoverMw.func1.1()
// 	D:/Gophercises/recovery/main.go:52 +0x8a
// panic({0x78b740, 0x964380})
// 	C:/Program Files/Go/src/runtime/panic.go:884 +0x213
// main.funcThatPanics(...)
// 	D:/Gophercises/recovery/main.go:129
// main.panicDemo({0x7c3e60?, 0x0?}, 0x1f85f1431d8?)
// 	D:/Gophercises/recovery/main.go:120 +0x27
// net/http.HandlerFunc.ServeHTTP(0xc000080000?, {0x966f00?, 0xc00026a090?}, 0xc0005bba10?)
// 	C:/Program Files/Go/src/net/http/server.go:2122 +0x2f
// net/http.(*ServeMux).ServeHTTP(0x35de46?, {0x966f00, 0xc00026a090}, 0xc000296300)
// 	C:/Program Files/Go/src/net/http/server.go:2500 +0x149
// main.recoverMw.func1({0x9670e0?, 0xc0002a8000}, 0x7ac9e0?)
// 	D:/Gophercises/recovery/main.go:64 +0xf9
// net/http.HandlerFunc.ServeHTTP(0x0?, {0x9670e0?, 0xc0002a8000?}, 0x3030d3?)
// 	C:/Program Files/Go/src/net/http/server.go:2122 +0x2f
// net/http.serverHandler.ServeHTTP({0xc0001de060?}, {0x9670e0, 0xc0002a8000}, 0xc000296300)
// 	C:/Program Files/Go/src/net/http/server.go:2936 +0x316
// net/http.(*conn).serve(0xc000160240, {0x967378, 0xc0001df470})
// 	C:/Program Files/Go/src/net/http/server.go:1995 +0x612
// created by net/http.(*Server).Serve
// 	C:/Program Files/Go/src/net/http/server.go:3089 +0x5ed
func makeLinks(trace string) string {
	lines := strings.Split(trace, "\n")
	for li, line := range lines {
		if len(line) == 0 || line[0] != '\t' {
			continue
		}
		file := ""
		count := 0
		for i, ch := range line {
			if ch == ':' {
				count += 1
				if count > 1 {
					file = line[1:i]
					break
				}
			}
		}
		v := url.Values{}
		v.Set("path", file)
		lines[li] = "\t<a href=\"/debug/?path=" + v.Encode() + "\">" + file + "</a>" + line[len(file)+1:]
	}
	return strings.Join(lines, "\n")
}