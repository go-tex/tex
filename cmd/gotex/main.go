// Command gotex processes a LaTeX-subset document (file argument or stdin) and
// writes an HTML document to stdout.
package main

import (
	"fmt"
	"io"
	"os"

	tex "github.com/go-tex/tex"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var src []byte
	var err error
	if len(args) > 0 {
		src, err = os.ReadFile(args[0])
	} else {
		src, err = io.ReadAll(stdin)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gotex: %v\n", err)
		return 1
	}
	html, err := tex.RenderHTML(string(src))
	if err != nil {
		fmt.Fprintf(stderr, "gotex: %v\n", err)
		return 1
	}
	io.WriteString(stdout, html)
	return 0
}
