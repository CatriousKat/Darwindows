package main

import (
	"fmt"
	"os"
	"darwindows/translation"
)

func main() {
	args := os.Args[1:]
	var filePath string

	for _, arg := range args {
		if arg == "--debug" {
			translation.Debug = true
		} else if filePath == "" {
			filePath = arg
		}
	}

	if filePath == "" {
		fmt.Println("Usage: darwindows.exe [--debug] <path_to_macho>")
		os.Exit(1)
	}

	if err := translation.RunMachO(filePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}