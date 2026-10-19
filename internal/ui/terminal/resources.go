// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"fmt"
	"github.com/bfcamel/CamelClip/internal/application"
	"github.com/bfcamel/CamelClip/internal/core"
	"strconv"
	"strings"
)

func resourceTitle(key string) string {
	return tr(map[string]string{"downloads": "Одновременных скачиваний (1–32)", "processing-limit": "Обработок (auto или 1–32)", "global-rate": "Бюджет скорости: фиксированные доли, новые загрузки", "job-rate": "Предел на задание: новые загрузки", "uploads": "Одновременных S3-отправок (1–32)", "upload-parts": "Частей на S3-отправку (1–4)", "retry-attempts": "Автоповторы (0 — выключены, максимум 10)", "staging-files": "Файлов в staging (0 — без порога)", "staging-bytes": "Байт в staging (0 — без порога)", "min-free": "Свободно байт на диске (0 — без порога)", "upload-rate": "Скорость S3 на отправку (0 — без предела)"}[key])
}
func resourceValue(o options, key string) string {
	switch key {
	case "uploads":
		return strconv.Itoa(uploadOptions(o).Objects)
	case "upload-parts":
		return strconv.Itoa(uploadOptions(o).Parts)
	case "retry-attempts":
		return strconv.Itoa(o.RetryAttempts)
	case "staging-files":
		return strconv.Itoa(o.StagingFiles)
	case "staging-bytes":
		return strconv.FormatInt(o.StagingBytes, 10)
	case "min-free":
		return strconv.FormatInt(o.MinFreeBytes, 10)
	case "upload-rate":
		return strconv.FormatInt(o.UploadRateBPS, 10)
	case "downloads":
		return strconv.Itoa(settingsFor(o).Limits().Effective().Downloads)
	case "processing-limit":
		if o.MaxProcessing == 0 {
			return "auto"
		}
		return strconv.Itoa(o.MaxProcessing)
	case "global-rate":
		return strconv.FormatInt(o.RateLimitBPS, 10)
	case "job-rate":
		return strconv.FormatInt(o.JobRateLimitBPS, 10)
	}
	return ""
}
func setResource(o *options, key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "uploads", "upload-parts", "retry-attempts", "staging-files", "staging-bytes", "min-free", "upload-rate":
		n, err := strconv.ParseInt(value, 10, 64)
		if key == "upload-rate" {
			n, err = core.ParseRate(value)
		}
		if err != nil || n < 0 {
			return fmt.Errorf("nonnegative integer required")
		}
		switch key {
		case "uploads":
			if n < 1 || n > 32 {
				return fmt.Errorf("1..32")
			}
			o.MaxUploads = int(n)
		case "upload-parts":
			if n < 1 || n > 4 {
				return fmt.Errorf("1..4")
			}
			o.UploadParts = int(n)
		case "retry-attempts":
			if n > 10 {
				return fmt.Errorf("0..10")
			}
			o.RetryAttempts = int(n)
		case "staging-files":
			if n > 100000 {
				return fmt.Errorf("0..100000")
			}
			o.StagingFiles = int(n)
		case "staging-bytes":
			o.StagingBytes = n
		case "min-free":
			o.MinFreeBytes = n
		case "upload-rate":
			o.UploadRateBPS = n
		}
	case "downloads", "processing-limit":
		if value == "auto" && key == "processing-limit" {
			o.MaxProcessing = 0
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 32 {
			return fmt.Errorf("1..32; processing: auto")
		}
		if key == "downloads" {
			o.MaxDownloads = n
		} else {
			o.MaxProcessing = n
		}
	case "global-rate", "job-rate":
		n, err := core.ParseRate(value)
		if err != nil {
			return err
		}
		if key == "global-rate" {
			o.RateLimitBPS = n
		} else {
			o.JobRateLimitBPS = n
		}
	}
	return settingsFor(*o).Limits().Validate()
}

func resourceDisplay(o options, key string) string {
	if key == "processing-limit" && o.MaxProcessing == 0 {
		return fmt.Sprintf("auto (%d)", settingsFor(o).Limits().Effective().Processing)
	}
	return resourceValue(o, key)
}

var extraResourceKeys = []string{"uploads", "upload-parts", "retry-attempts", "staging-files", "staging-bytes", "min-free", "upload-rate"}

func queuePolicy(o options) application.QueuePolicy {
	return application.QueuePolicy{RetryAttempts: o.RetryAttempts, PendingFiles: o.StagingFiles, PendingBytes: o.StagingBytes, MinFreeBytes: o.MinFreeBytes}
}
func uploadOptions(o options) core.UploadOptions {
	objects, parts := o.MaxUploads, o.UploadParts
	if objects == 0 {
		objects = 2
	}
	if parts == 0 {
		parts = 2
	}
	return core.UploadOptions{Objects: objects, Parts: parts, RetryAttempts: o.RetryAttempts, RateBPS: o.UploadRateBPS}
}
