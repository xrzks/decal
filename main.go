package main

import (
	"context"
	"log"
	"os"

	"templ/internal/cli"
)

func main() {
	log.SetFlags(0)
	if err := cli.New().Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
