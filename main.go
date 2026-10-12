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
		os.Exit(1)
	}
}
