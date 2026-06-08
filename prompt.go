package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var stdin = bufio.NewReader(os.Stdin)

// ask prompts for a line, returning def if the user just hits enter.
func ask(prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, _ := stdin.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return def
	}
	return line
}

// askYesNo prompts for a yes/no answer, returning def on a bare enter.
func askYesNo(prompt string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Printf("%s [%s]: ", prompt, hint)
	line, _ := stdin.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def
	case "y", "yes", "o", "oui":
		return true
	default:
		return false
	}
}

// askInt prompts until it gets a valid integer, returning def on a bare enter.
func askInt(prompt string, def int) int {
	for {
		s := ask(prompt, strconv.Itoa(def))
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		fmt.Println("  valeur numérique attendue")
	}
}
