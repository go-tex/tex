// Copyright (c) the go-tex/tex authors.
// SPDX-License-Identifier: BSD-3-Clause

package tex

import (
	"html"
	"strings"

	texmath "github.com/go-tex/math"
	"golang.org/x/text/unicode/norm"
)

// mathRenderer typesets math with the embedded STIX Two Math font; the embedded
// font always carries a MATH table, so construction never fails.
var mathRenderer, _ = texmath.New(texmath.DefaultFont())

// RenderHTML processes a LaTeX-subset document and returns a complete HTML
// document (with math typeset to inline SVG). The error result is reserved for
// future fatal-processing conditions; today it is always nil.
func RenderHTML(src string) (string, error) {
	p := &proc{m: newMachine(tokenize(src)), math: mathRenderer}
	p.preamble()
	p.body()
	p.closePara()
	return p.wrap(), nil
}

type proc struct {
	m    *machine
	math *texmath.Renderer
	out  strings.Builder

	title, author, date string
	inPara              bool
	itemOpen            bool
	listDepth           int
}

// ── preamble ────────────────────────────────────────────────────────────────

func (p *proc) preamble() {
	for {
		t, ok := p.m.next()
		if !ok {
			return
		}
		if t.typ == tCS {
			switch t.s {
			case "documentclass":
				p.skipOptional()
				p.grabGroupText()
			case "usepackage":
				p.skipOptional()
				p.grabGroupText()
			case "title":
				p.title = p.grabInline()
			case "author":
				p.author = p.grabInline()
			case "date":
				p.date = p.grabInline()
			case "begin":
				if p.grabGroupText() == "document" {
					return
				}
			}
		}
	}
}

// ── body ────────────────────────────────────────────────────────────────────

func (p *proc) body() {
	for {
		t, ok := p.m.next()
		if !ok {
			return
		}
		switch t.typ {
		case tPar:
			p.closePara()
		case tSpace:
			if p.inPara {
				p.out.WriteByte(' ')
			}
		case tChar:
			p.text(string(t.r))
		case tMath:
			p.math2(t.s == "$$")
		case tOpen, tClose:
			// bare grouping in text: transparent
		case tCS:
			if p.command(t.s) {
				return // \end{document}
			}
		}
	}
}

// command dispatches a control sequence in body text. Returns true to stop
// (\end{document}).
func (p *proc) command(name string) bool {
	switch name {
	case "end":
		env := p.grabGroupText()
		if env == "document" {
			return true
		}
		p.endEnv(env)
	case "begin":
		p.beginEnv(p.grabGroupText())
	case "section", "section*":
		p.heading("h2", p.grabInline())
	case "subsection", "subsection*":
		p.heading("h3", p.grabInline())
	case "subsubsection", "subsubsection*":
		p.heading("h4", p.grabInline())
	case "paragraph":
		p.heading("h5", p.grabInline())
	case "maketitle":
		p.maketitle()
	case "textbf", "bf":
		p.wrapInline("strong", p.grabInline())
	case "textit", "emph", "it", "em":
		p.wrapInline("em", p.grabInline())
	case "texttt", "tt", "verb":
		p.wrapInline("code", p.grabInline())
	case "underline":
		p.wrapInline("u", p.grabInline())
	case "textsc":
		p.rawInline(`<span style="font-variant:small-caps">` + html.EscapeString(p.grabInline()) + `</span>`)
	case "item":
		p.item()
	case "\\", "newline":
		if p.inPara {
			p.out.WriteString("<br>")
		}
	case "[":
		p.displayMathUntil("]")
	case "(":
		p.inlineMathUntil(")")
	case "LaTeX":
		p.rawInline("LaTeX")
	case "TeX":
		p.rawInline("TeX")
	case "ldots", "dots":
		p.text("…")
	case "%", "&", "_", "$", "#", "{", "}":
		p.text(name)
	case " ", "quad", "qquad", ",", ";", ":", "!":
		if p.inPara {
			p.out.WriteByte(' ')
		}
	default:
		if acc, ok := textAccents[name]; ok {
			p.text(p.applyAccent(acc))
		}
		// unknown commands are silently dropped (best-effort subset)
	}
	return false
}

// ── environments ────────────────────────────────────────────────────────────

func (p *proc) beginEnv(env string) {
	switch env {
	case "itemize":
		p.closePara()
		p.out.WriteString("<ul>")
		p.listDepth++
	case "enumerate":
		p.closePara()
		p.out.WriteString("<ol>")
		p.listDepth++
	case "quote", "quotation":
		p.closePara()
		p.out.WriteString("<blockquote>")
	case "center":
		p.closePara()
		p.out.WriteString(`<div style="text-align:center">`)
	case "equation", "equation*", "displaymath", "align", "align*":
		p.displayMathUntilEnd(env)
	case "verbatim":
		p.verbatim()
	}
}

func (p *proc) endEnv(env string) {
	switch env {
	case "itemize":
		p.closeItem()
		p.out.WriteString("</ul>")
		p.listDepth--
	case "enumerate":
		p.closeItem()
		p.out.WriteString("</ol>")
		p.listDepth--
	case "quote", "quotation":
		p.closePara()
		p.out.WriteString("</blockquote>")
	case "center":
		p.closePara()
		p.out.WriteString("</div>")
	}
}

func (p *proc) item() {
	p.closeItem()
	p.out.WriteString("<li>")
	p.itemOpen = true
}

func (p *proc) closeItem() {
	if p.itemOpen {
		p.out.WriteString("</li>")
		p.itemOpen = false
	}
}

// ── math ────────────────────────────────────────────────────────────────────

// math2 collects source up to the closing $ (or $$) and typesets it.
func (p *proc) math2(display bool) {
	src := p.collectMath(func(t tok) bool { return t.typ == tMath })
	p.emitMath(src, display)
}

func (p *proc) inlineMathUntil(closeSym string) {
	src := p.collectMath(func(t tok) bool { return t.typ == tCS && t.s == closeSym })
	p.emitMath(src, false)
}

func (p *proc) displayMathUntil(closeSym string) {
	src := p.collectMath(func(t tok) bool { return t.typ == tCS && t.s == closeSym })
	p.emitMath(src, true)
}

func (p *proc) displayMathUntilEnd(env string) {
	src := p.collectMath(func(t tok) bool {
		if t.typ == tCS && t.s == "end" {
			p.grabGroupText() // consume {env}
			return true
		}
		return false
	})
	p.emitMath(src, true)
}

// collectMath reconstructs math source from tokens until stop matches.
func (p *proc) collectMath(stop func(tok) bool) string {
	var b strings.Builder
	for {
		t, ok := p.m.next()
		if !ok || stop(t) {
			return b.String()
		}
		switch t.typ {
		case tChar:
			b.WriteRune(t.r)
		case tCS:
			b.WriteString("\\" + t.s)
			if isWord(t.s) {
				b.WriteByte(' ')
			}
		case tOpen:
			b.WriteByte('{')
		case tClose:
			b.WriteByte('}')
		case tSpace:
			b.WriteByte(' ')
		case tAmp:
			b.WriteByte('&')
		}
	}
}

func (p *proc) emitMath(src string, display bool) {
	src = strings.TrimSpace(src)
	if src == "" {
		return
	}
	var svg string
	var err error
	if display {
		svg, err = p.math.RenderDisplaySVG(src, 22)
	} else {
		svg, err = p.math.RenderSVG(src, 18)
	}
	if err != nil {
		p.rawInline(`<span class="math-error" title="` + html.EscapeString(err.Error()) + `">` + html.EscapeString(src) + `</span>`)
		return
	}
	if display {
		p.closePara()
		p.out.WriteString(`<div class="math-display" style="text-align:center;margin:1em 0">` + svg + `</div>`)
	} else {
		p.rawInline(`<span class="math" style="vertical-align:middle">` + svg + `</span>`)
	}
}

// ── output helpers ──────────────────────────────────────────────────────────

func (p *proc) text(s string) {
	p.openPara()
	p.out.WriteString(html.EscapeString(s))
}

func (p *proc) rawInline(h string) {
	p.openPara()
	p.out.WriteString(h)
}

func (p *proc) wrapInline(tag, content string) {
	p.rawInline("<" + tag + ">" + html.EscapeString(content) + "</" + tag + ">")
}

func (p *proc) heading(tag, content string) {
	p.closePara()
	p.out.WriteString("<" + tag + ">" + html.EscapeString(content) + "</" + tag + ">")
}

func (p *proc) openPara() {
	if !p.inPara && p.listDepth == 0 && !p.itemOpen {
		p.out.WriteString("<p>")
		p.inPara = true
	}
}

func (p *proc) closePara() {
	if p.inPara {
		p.out.WriteString("</p>")
		p.inPara = false
	}
}

func (p *proc) maketitle() {
	p.closePara()
	p.out.WriteString(`<header class="titleblock" style="text-align:center;margin:2em 0">`)
	if p.title != "" {
		p.out.WriteString("<h1>" + html.EscapeString(p.title) + "</h1>")
	}
	if p.author != "" {
		p.out.WriteString(`<div class="author">` + html.EscapeString(p.author) + "</div>")
	}
	if p.date != "" {
		p.out.WriteString(`<div class="date">` + html.EscapeString(p.date) + "</div>")
	}
	p.out.WriteString("</header>")
}

// verbatim reads raw tokens until \end{verbatim} and emits a <pre>.
func (p *proc) verbatim() {
	p.closePara()
	var b strings.Builder
	for {
		t, ok := p.m.raw()
		if !ok {
			break
		}
		if t.typ == tCS && t.s == "end" {
			p.grabGroupText()
			break
		}
		b.WriteString(tokRaw(t))
	}
	p.out.WriteString("<pre>" + html.EscapeString(b.String()) + "</pre>")
}

// ── argument helpers (on the processor's machine) ───────────────────────────

func (p *proc) grabInline() string {
	p.m.skipSpaces()
	t, ok := p.m.raw()
	if !ok {
		return ""
	}
	if t.typ != tOpen {
		return tokRaw(t)
	}
	return tokToString(p.m.grabGroup())
}

func (p *proc) grabGroupText() string {
	p.m.skipSpaces()
	t, ok := p.m.raw()
	if !ok || t.typ != tOpen {
		return ""
	}
	return tokToString(p.m.grabGroup())
}

func (p *proc) skipOptional() {
	p.m.skipSpaces()
	t, ok := p.m.raw()
	if !ok {
		return
	}
	if t.typ == tChar && t.r == '[' {
		for {
			u, ok := p.m.raw()
			if !ok || (u.typ == tChar && u.r == ']') {
				return
			}
		}
	}
	p.m.unraw(t)
}

// applyAccent combines a text accent command with its argument letter.
func (p *proc) applyAccent(combining rune) string {
	arg := p.grabInline()
	return norm.NFC.String(arg + string(combining)) // precompose é, ö, … when possible
}

func (p *proc) wrap() string {
	return `<!doctype html><meta charset="utf-8"><style>` + css + `</style><article>` + p.out.String() + `</article>`
}

func isWord(s string) bool {
	for _, r := range s {
		if !isLetter(r) {
			return false
		}
	}
	return s != ""
}

func tokRaw(t tok) string {
	switch t.typ {
	case tChar:
		return string(t.r)
	case tCS:
		return "\\" + t.s
	case tOpen:
		return "{"
	case tClose:
		return "}"
	case tSpace:
		return " "
	case tPar:
		return "\n\n"
	default:
		return ""
	}
}

// textAccents maps a text-mode accent command to a Unicode combining mark.
var textAccents = map[string]rune{
	"'": '́', "`": '̀', "^": '̂', "\"": '̈',
	"~": '̃', "=": '̄', ".": '̇', "u": '̆',
	"v": '̌', "c": '̧', "H": '̋',
}

const css = `article{max-width:46rem;margin:2rem auto;padding:0 1rem;font:16px/1.6 Georgia,serif;color:#111}` +
	`h1,h2,h3,h4{font-family:system-ui,sans-serif;line-height:1.2}` +
	`code{font-family:ui-monospace,monospace;background:#f4f4f4;padding:.1em .3em;border-radius:3px}` +
	`blockquote{border-left:3px solid #ccc;margin:1em 0;padding-left:1em;color:#444}` +
	`.math svg,.math-display svg{max-width:100%}` +
	`.math-error{color:#b00;background:#fee;padding:0 .2em}` +
	`@media(prefers-color-scheme:dark){article{color:#eee;background:#111}code{background:#222}}`
