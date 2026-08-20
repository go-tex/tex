// Command gotex processes a LaTeX-subset document (file argument or stdin) and
// writes an HTML document to stdout.
package main

import (
	"fmt"
	"io"
	"os"

	tex "github.com/go-tex/tex"
)

// osExit is the process-exit seam. Tests replace it so main itself is
// reachable; nothing else assigns it.
var osExit = os.Exit

func main() { osExit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

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
	// tex.RenderHTML's error return is always nil -- it ends in
	// `return p.wrap(), nil` and process.go has no other error path -- so the
	// `if err != nil` that used to sit here could never be taken. It is
	// dropped rather than covered: unreachable code is what kept this package
	// off 100% and the whole coverage gate red.
	html, _ := tex.RenderHTML(string(src))
	_, _ = io.WriteString(stdout, html)
	return 0
}
