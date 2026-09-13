package main

import (
	"flag"
	"fmt"
	"os"

	"darwindows/translation"
)

func main() {
	debugPtr := flag.Bool("debug", false, "Enable verbose debugging and trap logging")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: darwindows.exe <macho_binary> [--debug]")
		os.Exit(1)
	}

	translation.Debug = *debugPtr
	filePath := args[0]

	if err := translation.RunMachO(filePath); err != nil {
		fmt.Fprintf(os.Stderr, "[Darwindows Error] %v\n", err)
		os.Exit(1)
	}
}