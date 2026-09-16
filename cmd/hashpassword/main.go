// hashpassword prints a bcrypt hash for a password, for use as the
// UXDESIGNS_PASSWORD_HASH Lambda env var. The plaintext password is never
// stored anywhere — only the hash printed here should be saved.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/arushiahmed/arushiahmed-site-api/auth"
)

func main() {
	password := readPassword()

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash password:", err)
		os.Exit(1)
	}

	fmt.Println(hash)
}

func readPassword() string {
	if term.IsTerminal(int(syscall.Stdin)) {
		fmt.Fprint(os.Stderr, "Password: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read password:", err)
			os.Exit(1)
		}
		return string(bytePassword)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}
