// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import (
	"fmt"
	"github.com/bfcamel/video-downloader/internal/core"
	"strconv"
	"strings"
)

func resourceTitle(key string) string {
	return tr(map[string]string{"downloads": "Одновременных скачиваний (1–32)", "processing-limit": "Обработок (auto или 1–32)", "global-rate": "Бюджет скорости: фиксированные доли, новые загрузки", "job-rate": "Предел на задание: новые загрузки"}[key])
}
func resourceValue(o options, key string) string {
	switch key {
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
