// athena-hashpw generates bcrypt password hashes for use in athena-dhcpd config.
// Usage:
//
//	athena-hashpw
//	athena-hashpw -cost 12
//	echo 'mypassword' | athena-hashpw
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func main() {
	cost := flag.Int("cost", 10, "bcrypt cost factor (4-31, default 10)")
	flag.Parse()

	if *cost < bcrypt.MinCost || *cost > bcrypt.MaxCost {
		fmt.Fprintf(os.Stderr, "error: cost must be between %d and %d\n", bcrypt.MinCost, bcrypt.MaxCost)
		os.Exit(1)
	}

	var password string

	if flag.NArg() > 0 {
		password = flag.Arg(0)
	} else if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Reading from pipe/stdin
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			password = strings.TrimSpace(scanner.Text())
		}
		if password == "" {
			fmt.Fprintln(os.Stderr, "error: empty password from stdin")
			os.Exit(1)
		}
	} else {
		// Interactive — prompt with hidden input
		fmt.Fprint(os.Stderr, "Password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading password: %v\n", err)
			os.Exit(1)
		}
		password = string(pw)

		fmt.Fprint(os.Stderr, "Confirm:  ")
		pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading confirmation: %v\n", err)
			os.Exit(1)
		}
		if string(pw2) != password {
			fmt.Fprintln(os.Stderr, "error: passwords do not match")
			os.Exit(1)
		}
	}

	if password == "" {
		fmt.Fprintln(os.Stderr, "error: password must not be empty")
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), *cost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(hash))
}
