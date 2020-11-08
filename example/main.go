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
			TEMPLATE_DATA	string
		}{
			TEMPLATE_DATA:	"zipbox template test",
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
