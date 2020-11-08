# zipbox

ZipBox is a [Go](http://golang.org) package that allows you to append files into your main binary. It creates a virtual filesystem in read-only mode using zip archive with the boxes that you appended into your main binary.

## Installation
From `repository`
```bash
$ go get -v -u github.com/blacklat/zipbox/...
$ ls -la "$( go env GOPATH )/bin/zipbox"
```

From `source`
```bash
$ git clone https://github.com/blacklat/zipbox.git
$ cd zipbox
$ go build -o "$( go env GOPATH )/bin/zipbox" zipbox/main.go
$ ls -la "$( go env GOPATH )/bin/zipbox"
```

## Usage
Using `repository`
```bash
$ cp -R "$( go env GOPATH )/src/github.com/blacklat/zipbox/example" /tmp/zipbox-test
$ cd /tmp/zipbox-test
$ fallocate -l 10M public/test.bin
$ cat main.go
```
```go
package main

import (
    "fmt"
    "strings"
    "bytes"
    "net/http"
    "html/template"

    "github.com/blacklat/zipbox"
)

func main() {
    public, err := zipbox.Get("public")
    if err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    }

    private, err := zipbox.Get("private")
    if err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    }

    if file, err := public.String("public/test.txt"); err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    } else {
        fmt.Printf("[INFO] (PUBLIC) test.txt: %s\n", file)
    }

    if file, err := public.Bytes("public/test.bin"); err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    } else {
        fmt.Printf("[INFO] (PUBLIC) test.bin: len(%d)\n", len(file))
    }

    fmt.Printf("[INFO] listening on 127.0.0.1:8080\n")
    http.HandleFunc("/test.html", func(response http.ResponseWriter, request *http.Request) {
        test, err := private.String("private/test.tpl")
        if err != nil {
            fmt.Printf("[ERROR] %s\n", err)
            return
        }

        tpl, err := template.New("test.tpl").Funcs(template.FuncMap{
            "ToUpper": strings.ToUpper,
        }).Parse(test)
        if err != nil {
            fmt.Printf("[ERROR] %s\n", err)
            return
        }

        items := struct {
            TEMPLATE_DATA   string
        }{
            TEMPLATE_DATA:  "zipbox template test",
        }

        res := &bytes.Buffer{}
        if err := tpl.Execute(response, items); err != nil {
            fmt.Printf("[ERROR] %s\n", err)
            return
        }
        fmt.Fprint(response, res.String())
    })
    http.Handle("/", http.FileServer(public.HTTPZipBox()))
    http.ListenAndServe("127.0.0.1:8080", nil)
}
```
```bash
$ go build -o test
$ zipbox --bin-file test
$ ./test
[INFO] (PUBLIC) test.txt: text file test
[INFO] (PUBLIC) test.bin: len(10485760)
[INFO] listening on 127.0.0.1:8080
```

Using `source`
```bash
$ cd example
$ fallocate -l 10M public/test.bin
$ sed -i 's/github.com\/blacklat\/zipbox/\.\./g' main.go
$ cat main.go
```
```go
package main

import (
    "fmt"
    "strings"
    "bytes"
    "net/http"
    "html/template"

    ".."
)

func main() {
    public, err := zipbox.Get("public")
    if err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    }

    private, err := zipbox.Get("private")
    if err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    }

    if file, err := public.String("public/test.txt"); err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    } else {
        fmt.Printf("[INFO] (PUBLIC) test.txt: %s\n", file)
    }

    if file, err := public.Bytes("public/test.bin"); err != nil {
        fmt.Printf("[ERROR] %s\n", err)
        return
    } else {
        fmt.Printf("[INFO] (PUBLIC) test.bin: len(%d)\n", len(file))
    }

    fmt.Printf("[INFO] listening on 127.0.0.1:8080\n")
    http.HandleFunc("/test.html", func(response http.ResponseWriter, request *http.Request) {
        test, err := private.String("private/test.tpl")
        if err != nil {
            fmt.Printf("[ERROR] %s\n", err)
            return
        }

        tpl, err := template.New("test.tpl").Funcs(template.FuncMap{
            "ToUpper": strings.ToUpper,
        }).Parse(test)
        if err != nil {
            fmt.Printf("[ERROR] %s\n", err)
            return
        }

        items := struct {
            TEMPLATE_DATA   string
        }{
            TEMPLATE_DATA:  "zipbox template test",
        }

        res := &bytes.Buffer{}
        if err := tpl.Execute(response, items); err != nil {
            fmt.Printf("[ERROR] %s\n", err)
            return
        }
        fmt.Fprint(response, res.String())
    })
    http.Handle("/", http.FileServer(public.HTTPZipBox()))
    http.ListenAndServe("127.0.0.1:8080", nil)
}
```
```bash
$ go build -o test
$ zipbox --import-name zipbox --import-path ".." --bin-file test
$ ./test
[INFO] (PUBLIC) test.txt: text file test
[INFO] (PUBLIC) test.bin: len(10485760)
[INFO] listening on 127.0.0.1:8080
```

## Usage (zipbox packer)
```bash
$ zipbox
usage: zipbox [options]

options:
 -q,  --quiet       > suppresses all output content except for errors
 -sp, --search-path > directory path of your go files that call ZipBox
 -bt, --build-tags
 -in, --import-name > the name of your imported zipbox package
 -ip, --import-path > the path of your imported zipbox package
 -bf, --bin-file    > path of your go binary that will be appended with ZipBox

```
