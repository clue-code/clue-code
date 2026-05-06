package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/clue-code/clue-code/internal/version"
)

func runVersion(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	short := fs.Bool("short", false, "print only the semver string")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *short {
		fmt.Println(version.Version)
		return
	}
	fmt.Println(version.String())
}
