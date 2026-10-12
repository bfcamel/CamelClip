package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Russian source strings are stable message keys; yt-dlp's own diagnostics stay unchanged.
var currentLanguage = "ru"

func tr(message string) string {
	if currentLanguage == "en" {
		if translated, ok := english[message]; ok {
			return translated
		}
	}
	return message
}
func localeLanguage(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" || locale == "c" || locale == "posix" || strings.HasPrefix(locale, "c.") {
		return ""
	}
	if locale == "ru" || strings.HasPrefix(locale, "ru_") || strings.HasPrefix(locale, "ru-") || strings.HasPrefix(locale, "ru.") {
		return "ru"
	}
	return "en"
}
func systemLanguage() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := localeLanguage(os.Getenv(name)); value != "" {
			return value
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
		tokens := strings.FieldsFunc(string(out), func(r rune) bool { return strings.ContainsRune("()\", \n\t\r", r) })
		if len(tokens) > 0 {
			if value := localeLanguage(tokens[0]); value != "" {
				return value
			}
		}
	}
	return "en"
}
func useLanguage(preference, system string) {
	currentLanguage = preference
	if preference == "auto" || preference == "" {
		currentLanguage = system
	}
}
func requestedLanguage(args []string, fallback string) string {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-lang=") {
			fallback = strings.TrimPrefix(arg, "-lang=")
		} else if strings.HasPrefix(arg, "--lang=") {
			fallback = strings.TrimPrefix(arg, "--lang=")
		} else if (arg == "-lang" || arg == "--lang") && i+1 < len(args) {
			fallback = args[i+1]
		}
	}
	return fallback
}

var english = map[string]string{
	"Отменено.":                 "Canceled.",
	"Ошибка:":                   "Error:",
	"операция отменена":         "operation canceled",
	"нет доступных вариантов":   "no options available",
	"Номер":                     "Number",
	"Введите номер из списка.":  "Enter a number from the list.",
	" — ↑/↓, Enter; q — отмена": " — Up/Down, Enter; q to cancel",
	"Не удалось прочитать настройки; используются стандартные значения:": "Could not read settings; using defaults:",
	"Ссылка; параметр можно повторять":                                   "Video URL; repeat this flag to add more links",
	"Текстовый файл: одна ссылка в строке":                               "Text file: one URL per line",
	"video — видео со звуком; audio — только звук":                       "video — video with audio; audio — audio only",
	"Максимальная высота кадра или best":                                 "Maximum frame height, or best",
	"Папка сохранения":                                                   "Output directory",
	"safari, chrome, firefox, edge, brave (не запоминается)":             "safari, chrome, firefox, edge, brave (not saved)",
	"Показать доступные форматы без скачивания":                          "List available formats without downloading",
	"не удалось открыть список: %w":                                      "could not open URL list: %w",
	"файл списка не содержит ссылок":                                     "the list file contains no URLs",
	"MacDown — видео и аудио. Настройки запоминаются автоматически.":     "MacDown — video and audio. Settings are saved automatically.",
	"Вставьте одну или несколько ссылок через пробел":                    "Paste one or more URLs separated by spaces",
	"Что скачать?":    "What would you like to download?",
	"Видео со звуком": "Video with audio",
	"Только звук":     "Audio only",
	"Формат аудио":    "Audio format",
	"не найден yt-dlp. Установка: brew install yt-dlp ffmpeg deno": "yt-dlp was not found. Install: brew install yt-dlp ffmpeg deno",
	"не найден %s. Установка: brew install ffmpeg":                 "%s was not found. Install: brew install ffmpeg",
	"не удалось создать папку %s: %w":                              "could not create directory %s: %w",
	"Получение доступных качеств…":                                 "Fetching available qualities…",
	"Качество (максимальная высота)":                               "Quality (maximum frame height)",
	"Доступно:":    "Available:",
	"Скачивание →": "Downloading to →",
	"Не удалось:":  "Failed:",
	"Не удалось сохранить настройки:":                                          "Could not save settings:",
	"\nЗавершено: %d. Ошибок: %d.\n":                                           "\nCompleted: %d. Failed: %d.\n",
	"часть ссылок не обработана; причины указаны выше":                         "some URLs failed; see the explanations above",
	"некорректные настройки: %w":                                               "invalid settings: %w",
	"получение данных прервано или заняло больше 90 секунд":                    "metadata retrieval was interrupted or exceeded 90 seconds",
	"yt-dlp вернул непонятные данные; обновите его: brew upgrade yt-dlp":       "yt-dlp returned invalid metadata; update it: brew upgrade yt-dlp",
	"нужна ссылка на отдельный ролик, а не на плейлист":                        "please provide a single-video URL, not a playlist",
	"это прямая трансляция; очередь поддерживает завершённые ролики":           "this is a livestream; the queue supports completed videos",
	"Лучшее доступное":                                                         "Best available",
	"До %dp":                                                                   "Up to %dp",
	"Недостаточно места на диске. Освободите место или выберите другую папку.": "Not enough disk space. Free some space or choose another output directory.",
	"Нет доступа к файлу или папке. Проверьте права и выбранную папку.":        "Permission denied. Check file permissions and the output directory.",
	"Не удалось прочитать cookies браузера. Проверьте профиль и разрешения macOS или выберите другой браузер.": "Could not read browser cookies. Check the profile and macOS permissions, or choose another browser.",
	"Ролик требует входа. Войдите в браузере и повторите с -cookies-browser firefox (или chrome/safari).":      "This video requires sign-in. Sign in through your browser and retry with -cookies-browser firefox (or chrome/safari).",
	"Сайт временно ограничил запросы. Подождите и повторите загрузку.":                                         "The site is rate-limiting requests. Wait before trying again.",
	"Сайт отказал в доступе. Обновите yt-dlp; если ролик требует входа, укажите -cookies-browser.":             "The site denied access. Update yt-dlp; if sign-in is required, use -cookies-browser.",
	"Ролик недоступен в вашем регионе.":                                                                        "This video is unavailable in your region.",
	"Ролик удалён или недоступен. Проверьте ссылку и доступ в браузере.":                                       "This video was removed or is unavailable. Check the URL and access in your browser.",
	"Выбранное качество недоступно. Выберите другое качество или best.":                                        "The requested quality is unavailable. Choose another quality or best.",
	"Не удалось подключиться к сайту. Проверьте интернет и повторите попытку.":                                 "Could not connect to the site. Check your internet connection and retry.",
	"Эта ссылка не поддерживается. Укажите прямую ссылку на страницу ролика.":                                  "This URL is not supported. Provide a direct link to the video page.",
	"Видео защищено DRM и не может быть скачано этой программой.":                                              "This video is DRM-protected and cannot be downloaded by this program.",
	"\nПопробуйте обновить yt-dlp: brew upgrade yt-dlp":                                                        "\nTry updating yt-dlp: brew upgrade yt-dlp",
	"Загрузка не завершена. Проверьте сообщения выше; обновление: brew upgrade yt-dlp.":                        "The download failed. Check the messages above; update with: brew upgrade yt-dlp.",
	"нужна корректная ссылка http:// или https://":                                                             "a valid http:// or https:// URL is required",
	"mode должен быть video или audio":                                                                         "mode must be video or audio",
	"quality должен быть best или числом от 1 до 16384":                                                        "quality must be best or an integer from 1 to 16384",
	"неподдерживаемый аудиоформат":                                                                             "unsupported audio format",
	"неподдерживаемый браузер":                                                                                 "unsupported browser",
	"папка сохранения не может быть пустой":                                                                    "output directory cannot be empty",
	"after_move:Сохранено: %(filepath)s":                                                                       "after_move:Saved: %(filepath)s",
	"Язык":               "Language",
	"Системный":          "System default",
	"Язык: auto, ru, en": "Language: auto, ru, en",
	"язык должен быть auto, ru или en": "language must be auto, ru or en",
}
