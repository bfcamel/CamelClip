// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package application

import (
	"errors"
	"sync"
)

type Workspace struct {
	Queue     Queue
	Library   *LibraryService
	Cloud     *CloudService
	Monitor   *SystemMonitor
	Playlists *PlaylistService
	once      sync.Once
	err       error
}

func (w *Workspace) Close() error {
	w.once.Do(func() {
		if w.Playlists != nil {
			w.err = errors.Join(w.err, w.Playlists.Close())
		}
		if w.Cloud != nil {
			w.err = errors.Join(w.err, w.Cloud.Close())
		}
		if w.Library != nil {
			w.Library.Close()
		}
		if w.Monitor != nil {
			w.Monitor.Close()
		}
		if w.Queue != nil {
			w.err = errors.Join(w.err, w.Queue.Close())
		}
	})
	return w.err
}
