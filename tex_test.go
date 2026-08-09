// Copyright (c) the go-tex/tex authors.
// SPDX-License-Identifier: BSD-3-Clause

package tex

import (
	"strings"
	"testing"
)

func mustHTML(t *testing.T, src string) string {
	t.Helper()
	h, err := RenderHTML(src)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	return h
}

// doc wraps body in a minimal document.
func doc(body string) string {
	return `\begin{document}` + body + `\end{document}`
}

func TestStructure(t *testing.T) {
	h := mustHTML(t, `\documentclass[12pt]{article}
\usepackage[utf8]{inputenc}
\title{My Title}\author{Me}\date{Today}
\begin{document}
\maketitle
\section{One}
A \textbf{bold} and \emph{italic} and \texttt{mono} word.
\subsection{Two}
\begin{itemize}\item a \item b\end{itemize}
\begin{enumerate}\item x\end{enumerate}
\begin{quote}quoted\end{quote}
\end{document}`)
	for _, want := range []string{
		"<h1>My Title</h1>", `class="author"`, "Today",
		"<h2>One</h2>", "<h3>Two</h3>",
		"<strong>bold</strong>", "<em>italic</em>", "<code>mono</code>",
		"<ul><li>", "<ol><li>", "<blockquote>", "quoted", "</blockquote>", "<p>",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestMacros(t *testing.T) {
	cases := []struct{ src, want string }{
		{`\newcommand{\hi}{HELLO}` + doc(`\hi`), "HELLO"},
		{`\newcommand{\sq}[1]{[#1]}` + doc(`\sq{z}`), "[z]"},
		{`\newcommand{\g}[2][D]{#1-#2}` + doc(`\g{b} \g[a]{b}`), "D-b"},
		{`\newcommand{\g}[2][D]{#1-#2}` + doc(`\g[a]{b}`), "a-b"},
		{`\def\foo#1{<#1>}` + doc(`\foo{q}`), "&lt;q&gt;"},
		{`\def\a{X}\let\b\a` + doc(`\b`), "X"},
		{`\newcommand{\dbl}[1]{#1#1}` + doc(`\dbl{ab}`), "abab"},
		{`\renewcommand{\ttt}{T}` + doc(`\ttt`), "T"},
	}
	for _, c := range cases {
		h := mustHTML(t, c.src)
		if !strings.Contains(h, c.want) {
			t.Errorf("src=%q: missing %q\n got=%s", c.src, c.want, body(h))
		}
	}
}

func TestMacroInMath(t *testing.T) {
	h := mustHTML(t, `\newcommand{\R}{\mathbb{R}}`+doc(`$\R^n$ and \[ \R \]`))
	if strings.Count(h, "<svg") != 2 {
		t.Errorf("expected 2 math svgs, got %d", strings.Count(h, "<svg"))
	}
	if strings.Contains(h, `\R`) {
		t.Error("macro \\R not expanded inside math")
	}
}

func TestMath(t *testing.T) {
	h := mustHTML(t, doc(`inline $x^2$ and $$y$$ and \[ \frac{a}{b} \] and \( z \)
and \begin{equation} \sum_i a_i \end{equation}`))
	if n := strings.Count(h, "<svg"); n != 5 {
		t.Errorf("math svg count = %d, want 5", n)
	}
	if !strings.Contains(h, "math-display") {
		t.Error("no display math")
	}
	// a malformed math falls back to an error span, not a crash.
	he := mustHTML(t, doc(`$\frac{a}$`))
	if !strings.Contains(he, "math-error") {
		t.Error("bad math should produce math-error span")
	}
}

func TestSpecialsAndAccents(t *testing.T) {
	h := mustHTML(t, doc(`50\% \& \_ \$ \# a~b \ldots \LaTeX{} \'e \"o \^i line\\break`))
	// accents are emitted as base letter + combining mark (valid NFD Unicode).
	for _, want := range []string{"50%", "&amp;", "…", "LaTeX", "\u00e9", "\u00f6", "\u00ee", "<br>"} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in %s", want, body(h))
		}
	}
}

func TestEdgeCases(t *testing.T) {
	// empty and no-document sources don't crash.
	if _, err := RenderHTML(""); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderHTML(`just text no env`); err != nil {
		t.Fatal(err)
	}
	// unknown command is dropped; verbatim is raw; center/align.
	h := mustHTML(t, doc(`\unknowncmd text \begin{center}mid\end{center}
\begin{verbatim}
\raw{code} $notmath$
\end{verbatim}`))
	if !strings.Contains(h, "text") || !strings.Contains(h, "text-align:center") {
		t.Errorf("center/text missing: %s", body(h))
	}
	if !strings.Contains(h, "<pre>") || !strings.Contains(h, `\raw{code}`) {
		t.Errorf("verbatim not raw: %s", body(h))
	}
	// display math via align env, and a runaway macro is bounded.
	mustHTML(t, `\def\loop{\loop\loop}`+doc(`\loop x`))
	mustHTML(t, doc(`\begin{align} a &= b \end{align}`))
}

func TestMoreCoverage(t *testing.T) {
	cases := []string{
		`\def\foo x{Y}` + doc(`\foo`),                // \def with delimiter token in param text
		`\newcommand\bare{Z}` + doc(`\bare`),         // \newcommand bare (no braces)
		`\newcommand{\g}[1][\alpha]{#1}` + doc(`\g`), // optional default is a token list
		`\let\c=\d` + doc(`\c`),                      // \let to an undefined cs
		`\def\a{A}\let\b=\a` + doc(`\b`),             // \let with = to a macro
		doc(`\paragraph{P} \textsc{sc} \underline{u} \newline x`),
		doc(`item outside \item list`),                                         // \item with no list open
		doc(`\begin{foo}unknown env\end{foo}`),                                 // unknown environment
		doc(`\begin{displaymath} a \end{displaymath}`),                         // displaymath env
		doc(`empty $ $ inline and $$ $$ display`),                              // empty math is skipped
		doc(`\unknown \notacommand{arg}`),                                      // unknown commands dropped
		`\begin{verbatim}` + "\nline1\n\nline2\n" + `\end{verbatim}` + doc(``), // verbatim w/ blank line (preamble)
		`\title{T}` + doc(`\maketitle`),                                        // title only (no author/date)
	}
	for _, src := range cases {
		if _, err := RenderHTML(src); err != nil {
			t.Errorf("RenderHTML(%.30q): %v", src, err)
		}
	}
	// verbatim inside the document body with a paragraph break.
	h := mustHTML(t, doc("\\begin{verbatim}\na\n\nb\n\\end{verbatim}"))
	if !strings.Contains(h, "<pre>") {
		t.Error("verbatim not emitted")
	}
	// exercise the remaining command/branch coverage:
	mustHTML(t, `\usepackage{nobrackets}\newcommand{5}{bad}`+ // usepackage w/o [opt]; \newcommand{non-cs}
		doc("para one\n\npara two \\subsubsection{S} \\TeX{} word\\quad next $a \\, b$"))
	// a macro used at end-of-input with a missing mandatory / optional argument.
	mustHTML(t, `\newcommand{\m}[1]{[#1]}`+`\begin{document}\m`)    // grabArg hits EOF
	mustHTML(t, `\newcommand{\o}[1][D]{(#1)}`+`\begin{document}\o`) // grabOptional hits EOF
}

func TestTruncated(t *testing.T) {
	// truncated / malformed inputs must not panic; they exercise the many
	// end-of-input guards in the argument/definition readers.
	for _, src := range []string{
		`\newcommand`, `\newcommand{\x}`, `\newcommand{\x}[1]`, `\newcommand{\x}[1][`,
		`\newcommand{`, `\newcommand{x`, `\newcommand{\x}[2]{#1}` + doc(`\x{a}`),
		`\def`, `\def\x`, `\def\x#1`, `\def\x{`,
		`\let`, `\let\a`, `\let\a=`,
		doc(`\textbf`), doc(`\textbf{unclosed`), doc(`\section`), doc(`$x`), doc(`\[ a`),
		doc(`\'`), doc(`{`), doc(`\begin{itemize}\item a`), doc(`\begin`), doc(`\begin{`),
		doc(`\end`), doc(`\begin{verbatim}\nx`),
		`\begin{document}\section`, // \section grabInline hits EOF
		`\begin{document}\begin`,   // \begin grabGroupText hits EOF
		`\documentclass`,           // \documentclass skipOptional hits EOF
		`\newcommand 5{x}`,         // \newcommand name is a non-cs token
		`\begin{document}\begin{verbatim} raw text with no end`, // verbatim hits EOF
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			_, _ = RenderHTML(src)
		}()
	}
}

func TestArgReadersAtEOF(t *testing.T) {
	// direct white-box coverage of the end-of-input guards in the arg readers.
	p := &proc{m: newMachine(nil)}
	if p.grabInline() != "" {
		t.Error("grabInline at EOF should be empty")
	}
	if p.grabGroupText() != "" {
		t.Error("grabGroupText at EOF should be empty")
	}
	p.skipOptional() // must not panic at EOF
}

func TestTokenize(t *testing.T) {
	ts := tokenize("a % comment\nb\n\nc \\foo \\{ $x$ #1 ~ &")
	kinds := map[ttype]int{}
	for _, tk := range ts {
		kinds[tk.typ]++
	}
	if kinds[tPar] != 1 {
		t.Errorf("paragraph breaks = %d, want 1", kinds[tPar])
	}
	if kinds[tMath] != 2 {
		t.Errorf("math shifts = %d, want 2", kinds[tMath])
	}
	if kinds[tCS] != 2 { // \foo and \{
		t.Errorf("control seqs = %d, want 2", kinds[tCS])
	}
	if kinds[tParam] != 1 || kinds[tAmp] != 1 {
		t.Error("param/amp not tokenized")
	}
}

// body extracts the <article> inner HTML for readable failure messages.
func body(h string) string {
	i := strings.Index(h, "<article>")
	if i < 0 {
		return h
	}
	return h[i:]
}
