package main

import "os"

func main() {
	if len(os.Args) > 1 && os.Args[1] == accountsCommand {
		os.Exit(runAccounts(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
