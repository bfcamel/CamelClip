package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, errCanceled) {
			fmt.Fprintln(os.Stderr, tr("Отменено."))
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, tr("Ошибка:"), err)
		var usage *usageError
		if errors.As(err, &usage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
