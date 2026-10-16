// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"errors"
	"fmt"
	"os"
)

func Main(deps Dependencies) {
	if err := run(deps); err != nil {
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
