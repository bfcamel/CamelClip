// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/core"
	"sync"
)

// downloadWithContract serializes adapter callbacks and seals the event stream
// when Download returns, including panic unwinding. Queue state and resource
// admission belong to the caller; this boundary only enforces adapter contracts.
func downloadWithContract(ctx context.Context, backend core.Downloader, r core.Request, observe core.Observer) (core.Result, error) {
	var callbacks sync.Mutex
	accepting := true
	stage := core.Downloading
	published := ""
	var violation error
	callback := func(e core.Event) {
		callbacks.Lock()
		defer callbacks.Unlock()
		if !accepting || violation != nil {
			return
		}
		switch e.Kind {
		case core.StageChanged:
			if core.StageOrder(e.Stage) == 0 || core.StageOrder(e.Stage) < core.StageOrder(stage) {
				violation = core.ContractError("adapter emitted an invalid stage transition")
				return
			}
			stage = e.Stage
		case core.Progress:
			if core.StageOrder(e.Stage) == 0 {
				violation = core.ContractError("adapter emitted progress for an unknown stage")
				return
			}
			if e.Metrics != nil {
				violation = core.ValidateMetrics(*e.Metrics)
				if violation != nil {
					return
				}
			}
		case core.Published:
			violation = core.ValidateResult(r, core.Result{Path: e.Path})
			if violation != nil {
				return
			}
			if published != "" && published != e.Path {
				violation = core.ContractError("adapter published multiple results")
				return
			}
			published = e.Path
		case core.Log:
		default:
			violation = core.ContractError("adapter emitted an unknown event")
			return
		}
		observe(e)
	}
	var result core.Result
	var err error
	func() {
		defer func() { callbacks.Lock(); accepting = false; callbacks.Unlock() }()
		result, err = backend.Download(ctx, r, callback)
	}()
	callbacks.Lock()
	defer callbacks.Unlock()
	if violation != nil {
		return result, violation
	}
	if err == nil {
		err = core.ValidateResult(r, result)
		if err == nil && published != "" && result.Path != published {
			err = core.ContractError("adapter result differs from its published path")
		}
	}
	return result, err
}
