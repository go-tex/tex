# tex — go-tex

[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests)

**A pure-Go (no cgo) processor for a practical subset of LaTeX _documents_.** It
does more than render math: it tokenizes with TeX category codes, **expands user
macros** (`\def`, `\newcommand`/`\renewcommand` with mandatory _and_ optional
arguments, `\let`), parses **document structure**, and emits **semantic HTML**
with math typeset to inline SVG by
[go-tex/math](https://github.com/go-tex/math). No TeX engine binary, no cgo, no
server; it compiles to `GOOS=js/wasm`.

```go
html, _ := tex.RenderHTML(`\documentclass{article}
\newcommand{\R}{\mathbb{R}}
\title{Demo}\author{go-tex}
\begin{document}
\maketitle
\section{Intro}
A \textbf{bold} claim about $\R^n$ and a display:
\[ \int_0^\infty e^{-x^2}\,dx = \frac{\sqrt{\pi}}{2} \]
\begin{itemize}\item first \item second\end{itemize}
\end{document}`)
```

## What it processes

- **Macros** — `\newcommand{\name}[n][opt]{body}` (mandatory + one optional arg),
  `\renewcommand`, `\providecommand`, TeX `\def\name#1#2{body}`, and `\let`.
  Macros expand everywhere, including inside math.
- **Structure** — `\documentclass`/`\usepackage` (consumed), `\title`/`\author`/
  `\date`/`\maketitle`, `\section`/`\subsection`/`\subsubsection`/`\paragraph`,
  paragraphs (blank lines).
- **Inline formatting** — `\textbf`, `\textit`/`\emph`, `\texttt`, `\underline`,
  `\textsc`, `\\`/`\newline`.
- **Lists & blocks** — `itemize`, `enumerate`, `\item`, `quote`, `center`,
  `verbatim`.
- **Math** — `$…$`, `\(…\)` (inline) and `$$…$$`, `\[…\]`, `equation`,
  `displaymath`, `align` (display), typeset to SVG; a malformed formula degrades
  to an error span rather than failing the document.
- **Text niceties** — `\%` `\&` `\_` `\$` `\#` `\{` `\}`, `~`, `\ldots`, and text
  accents (`\'e`→é, `\"o`→ö, …, precomposed to NFC).

## Scope — honest limits

This is the **front of a TeX engine** (the "mouth" + a document builder), **not
a replacement for a TeX distribution (TeXLive)**. There is no page/line breaking,
no float/table/figure model, no counters/cross-references/bibliography, no
package ecosystem, and **no PDF/DVI back-end** — the output is HTML. Unknown
commands are dropped best-effort. Full LaTeX (real line-breaking, packages, PDF)
is the larger ongoing [go-tex](https://github.com/go-tex) engine effort; this
package is a working, useful foundation and a proof that *processing* documents
(not just rendering math) is tractable in pure Go.

## Install

```sh
go get github.com/go-tex/tex
```

## Command

`cmd/gotex` reads a `.tex` file (or stdin) and writes an HTML document:

```sh
go run ./cmd/gotex paper.tex > paper.html
```

## Tests

Statement coverage is held at **100%** (tokenizer, macro expander, document
processor, and every end-of-input guard), `go vet` clean, and green across the
six 64-bit Go targets plus `js/wasm` and `wasip1/wasm`.

```sh
go test ./...
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-tex/tex authors. The
embedded math font (via go-tex/math) is STIX Two Math, under the SIL OFL.
