package main

import (
	"fmt"
	"os"

	"github.com/pooh64/csif-driver/pkg/csif"
)

func main() {
	driver, err := csif.NewCsifDriver("csif-driver", "", "", "dummy-version")
	if err != nil {
		fmt.Printf("Can't create new driver: %s", err.Error())
		os.Exit(1)
	}

	if err := driver.Run(); err != nil {
		fmt.Printf("Failed to start driver: %s", err.Error())
		os.Exit(1)
	}
}
