package main

import (
	"os"

	"github.com/Geogboe/rog/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
