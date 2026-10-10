package main

import (
	"fmt"
	"os"

	"github.com/aymanbagabas/gh-notifs/internal/config"
)

func main() {
	if _, err := fmt.Print(config.DefaultText); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
