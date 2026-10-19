// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"github.com/bfcamel/CamelClip/internal/config"
	"github.com/bfcamel/CamelClip/internal/storage/jsonconfig"
)

var testDependencies = Dependencies{OpenConfig: func(path string, defaults config.Settings) config.Repository {
	return jsonconfig.File{Path: path, Defaults: defaults}
}}

func testOptions(home string) options {
	o := defaultOptions(home)
	o.runtime = &testDependencies
	return o
}
