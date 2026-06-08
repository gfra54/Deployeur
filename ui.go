package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const uiWidth = 66

// header prints a section separator bar with a title, to visually break up the
// setup output into phases.
func header(title string) {
	t := "── " + title + " "
	pad := uiWidth - utf8.RuneCountInString(t)
	if pad < 0 {
		pad = 0
	}
	fmt.Println("\n" + t + strings.Repeat("─", pad))
}

// box prints lines inside an ASCII box, with title in the top border. The box
// widens to fit the longest line (or the title).
func box(title string, lines []string) {
	inner := utf8.RuneCountInString(title) + 2
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n+2 > inner {
			inner = n + 2
		}
	}

	top := "┌"
	if title != "" {
		seg := "─ " + title + " "
		top += seg + strings.Repeat("─", inner-utf8.RuneCountInString(seg))
	} else {
		top += strings.Repeat("─", inner)
	}
	fmt.Println(top + "┐")
	for _, l := range lines {
		pad := inner - utf8.RuneCountInString(l) - 2
		fmt.Println("│ " + l + strings.Repeat(" ", pad) + " │")
	}
	fmt.Println("└" + strings.Repeat("─", inner) + "┘")
}
