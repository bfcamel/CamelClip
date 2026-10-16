// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"os"
	"strings"
	"sync"
)

// Russian source strings are stable message keys; yt-dlp's own diagnostics stay unchanged.
var currentLanguage = "ru"
var languageMu sync.RWMutex

func tr(message string) string {
	languageMu.RLock()
	defer languageMu.RUnlock()
	if currentLanguage == "en" {
		if translated, ok := english[message]; ok {
			return platformMessage(translated)
		}
	}
	return platformMessage(message)
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
	return preferredSystemLanguage()
}
func useLanguage(preference, system string) {
	languageMu.Lock()
	defer languageMu.Unlock()
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
	"Очередь хранится в SQLite.": "The queue is stored in SQLite.",
	"← Назад":  "← Back",
	"Отменить": "Cancel",
	"Повтор":   "Retry",
	"Убрать":   "Remove",
	"↑/↓: выбор  Enter: подтвердить  Esc/←: назад": "↑/↓: select  Enter: confirm  Esc/←: back",
	"0 / b — назад": "0 / b — back",
	"Видео со звуком в выбранном формате": "Video with audio in the selected container",
	"Кодек видео":  "Video codec",
	"Формат видео": "Video container",
	"Загрузка":     "Downloading",
	"Обработка":    "Processing",
	"Проверка":     "Verifying",
	"неподдерживаемый видеокодек: %s":                                         "unsupported video codec: %s",
	"неподдерживаемый видеоформат: %s":                                        "unsupported video container: %s",
	"WebM поддерживает только VP9 или AV1":                                    "WebM supports VP9 or AV1 only",
	"Очередь уже открыта в другом окне MacDown.":                              "The queue is already open in another MacDown window.",
	"Не удалось прочитать очередь; файл сохранён без изменений: %w":           "Cannot read the queue; file kept unchanged: %w",
	"Не удалось сохранить очередь: %w":                                        "Cannot save the queue: %w",
	"Сохранённая очередь":                                                     "Saved queue",
	"Очередь пуста.":                                                          "The queue is empty.",
	"Продолжить незавершённые":                                                "Resume unfinished jobs",
	"Повторить ошибки и отменённые":                                           "Retry failed and canceled jobs",
	"Убрать завершённые записи":                                               "Clear finished history",
	"Запись удалена из очереди. Файлы сохранены.":                             "Queue entry removed. Downloaded files are kept.",
	"x: отменить  r: повтор  d: убрать запись  o: Finder  f: папка  q: выход": "x: cancel  r: retry  d: remove entry  o: Finder  f: folder  q: quit",

	"Браузер cookies": "Browser cookies",
	"Браузер cookies: firefox[:профиль], пусто — отключить": "Browser cookies: firefox[:profile], empty to disable",
	"Проверка и подготовка совместимого MP4…":               "Preparing compatible MP4…",
	"Проверка целостности файла…":                           "Verifying media integrity…",
	"YouTube просит подтвердить, что вы не бот. Проверьте cookies выбранного профиля Firefox и доступ через ту же сеть.": "YouTube requests a bot check. Check the selected Firefox profile cookies and use the same network.",
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
	"Не удалось сохранить настройки:":                                                         "Could not save settings:",
	"\nЗавершено: %d. Ошибок: %d.\n":                                                          "\nCompleted: %d. Failed: %d.\n",
	"часть ссылок не обработана; причины указаны выше":                                        "some URLs failed; see the explanations above",
	"некорректные настройки: %w":                                                              "invalid settings: %w",
	"получение данных прервано или заняло больше 90 секунд":                                   "metadata retrieval was interrupted or exceeded 90 seconds",
	"yt-dlp вернул непонятные данные; обновите его: обновите yt-dlp вашим менеджером пакетов": "yt-dlp returned invalid metadata; update yt-dlp using your package manager",
	"нужна ссылка на отдельный ролик, а не на плейлист":                                       "please provide a single-video URL, not a playlist",
	"это прямая трансляция; очередь поддерживает завершённые ролики":                          "this is a livestream; the queue supports completed videos",
	"Лучшее доступное": "Best available",
	"До %dp":           "Up to %dp",
	"Недостаточно места на диске. Освободите место или выберите другую папку.":                                   "Not enough disk space. Free some space or choose another output directory.",
	"Нет доступа к файлу или папке. Проверьте права и выбранную папку.":                                          "Permission denied. Check file permissions and the output directory.",
	"Не удалось прочитать cookies браузера. Проверьте профиль и разрешения доступа или выберите другой браузер.": "Could not read browser cookies. Check the profile and access permissions, or choose another browser.",
	"Ролик требует входа. Войдите в браузере и повторите с -cookies-browser firefox (или chrome).":               "This video requires sign-in. Sign in through your browser and retry with -cookies-browser firefox (or chrome).",
	"Сайт временно ограничил запросы. Подождите и повторите загрузку.":                                           "The site is rate-limiting requests. Wait before trying again.",
	"Сайт отказал в доступе. Обновите yt-dlp; если ролик требует входа, укажите -cookies-browser.":               "The site denied access. Update yt-dlp; if sign-in is required, use -cookies-browser.",
	"Ролик недоступен в вашем регионе.":                                                                          "This video is unavailable in your region.",
	"Ролик удалён или недоступен. Проверьте ссылку и доступ в браузере.":                                         "This video was removed or is unavailable. Check the URL and access in your browser.",
	"Выбранное качество недоступно. Выберите другое качество или best.":                                          "The requested quality is unavailable. Choose another quality or best.",
	"Не удалось подключиться к сайту. Проверьте интернет и повторите попытку.":                                   "Could not connect to the site. Check your internet connection and retry.",
	"Эта ссылка не поддерживается. Укажите прямую ссылку на страницу ролика.":                                    "This URL is not supported. Provide a direct link to the video page.",
	"Видео защищено DRM и не может быть скачано этой программой.":                                                "This video is DRM-protected and cannot be downloaded by this program.",
	"\nОбновите yt-dlp вашим менеджером пакетов.":                                                                "\nTry updating yt-dlp using your package manager",
	"Загрузка не завершена. Проверьте сообщения выше и обновите yt-dlp вашим менеджером пакетов.":                "The download failed. Check the messages above; update yt-dlp using your package manager.",
	"нужна корректная ссылка http:// или https://":                                                               "a valid http:// or https:// URL is required",
	"mode должен быть video или audio":                  "mode must be video or audio",
	"quality должен быть best или числом от 1 до 16384": "quality must be best or an integer from 1 to 16384",
	"неподдерживаемый аудиоформат":                      "unsupported audio format",
	"неподдерживаемый браузер":                          "unsupported browser",
	"папка сохранения не может быть пустой":             "output directory cannot be empty",
	"after_move:Сохранено: %(filepath)s":                "after_move:Saved: %(filepath)s",
	"Язык":               "Language",
	"Системный":          "System default",
	"Язык: auto, ru, en": "Language: auto, ru, en",
	"язык должен быть auto, ru или en":                                     "language must be auto, ru or en",
	"Неизвестная команда: %s. Справка: macdown help":                       "Unknown command: %s. See: macdown help",
	"Неизвестный параметр: %s. Справка: macdown help":                      "Unknown option: %s. See: macdown help",
	"Параметру %s нужно значение":                                          "Option %s requires a value",
	"Эта команда не принимает ссылки или дополнительные аргументы":         "This command does not accept URLs or extra arguments",
	"Укажите URL или --list FILE. Для меню запустите macdown без команды.": "Provide a URL or --list FILE. Run macdown without a command for the interactive menu.",
	"рекомендуется для YouTube":                                            "recommended for YouTube",
	"не найден": "not found",
	"не удалось проверить версию":                                                                    "could not check version",
	"Зависимости: brew install yt-dlp ffmpeg deno":                                                   "Dependencies: brew install yt-dlp ffmpeg deno",
	"Проверка версий не гарантирует доступ к сайтам; EJS: https://github.com/yt-dlp/yt-dlp/wiki/EJS": "Version checks do not guarantee site access; EJS: https://github.com/yt-dlp/yt-dlp/wiki/EJS",
	"отсутствуют или не работают обязательные зависимости":                                           "required dependencies are missing or not working",
	"Использование:\n  macdown                         Интерактивное меню\n  macdown download URL...         Видео со звуком в выбранном формате\n  macdown audio URL...            Только звук\n  macdown formats URL...          Доступные форматы\n  macdown config                  Настройки и путь к файлу (JSON)\n  macdown migrate                 Перенести очередь в SQLite\n  macdown doctor [--json]         Проверка зависимостей и хранилища\n  macdown version                 Версия программы\n  macdown help [COMMAND]          Справка\n\nПараметры загрузки (до или после URL):\n  --quality HEIGHT|best          Максимальная высота видео\n  --video-codec h264|hevc|vp9|av1 Кодек видео (по умолчанию h264)\n  --video-format mp4|mkv|webm     Контейнер видео (по умолчанию mp4)\n  --audio-format mp3|m4a|opus|flac|wav|best\n  --out DIR                      Папка сохранения\n  --url URL                      Добавить ссылку (можно повторять)\n  --list FILE                    Файл со ссылками, по одной в строке\n  --cookies-browser BROWSER      Авторизация через браузер\n\nОбщие параметры:\n  --lang auto|en|ru               Язык\n  --help, -h                     Справка\n  --version                      Версия\n\nПримеры:\n  macdown download 'URL' --quality 1080\n  macdown audio 'URL' --audio-format mp3\n  macdown download --list links.txt\n\nСтарый синтаксис macdown -url URL -mode audio также поддерживается.": "Usage:\n  macdown                         Interactive menu\n  macdown download URL...         Video with audio in the selected container\n  macdown audio URL...            Audio only\n  macdown formats URL...          Available formats\n  macdown config                  Settings and file location (JSON)\n  macdown migrate                 Migrate the queue to SQLite\n  macdown doctor [--json]         Check dependencies and storage\n  macdown version                 Program version\n  macdown help [COMMAND]          Help\n\nDownload options (before or after URLs):\n  --quality HEIGHT|best          Maximum video height\n  --video-codec h264|hevc|vp9|av1 Video codec (default h264)\n  --video-format mp4|mkv|webm     Video container (default mp4)\n  --audio-format mp3|m4a|opus|flac|wav|best\n  --out DIR                      Output directory\n  --url URL                      Add a URL (repeatable)\n  --list FILE                    Text file: one URL per line\n  --cookies-browser BROWSER      Use browser authentication\n\nCommon options:\n  --lang auto|en|ru               Language\n  --help, -h                     Help\n  --version                      Version\n\nExamples:\n  macdown download 'URL' --quality 1080\n  macdown audio 'URL' --audio-format mp3\n  macdown download --list links.txt\n\nThe legacy syntax macdown -url URL -mode audio is also supported.",
	"Мои файлы":                       "My files",
	"Файлов: %d · Страница %d/%d\n":   "Files: %d · Page %d/%d\n",
	"Здесь пока нет видео или аудио.": "No video or audio files here yet.",
	"Следующая страница →":            "Next page →",
	"← Предыдущая страница":           "← Previous page",
	"Открыть папку":                   "Open folder",
	"Обновить список":                 "Refresh list",
	"← Главное меню":                  "← Main menu",
	"Выберите файл или действие":      "Select a file or action",
	"Файл":                              "File",
	"Действие":                          "Action",
	"Открыть файл":                      "Open file",
	"Открыть папку файла":               "Open containing folder",
	"← К списку файлов":                 "← Back to files",
	"Главное меню":                      "Main menu",
	"В библиотеке: %d файлов · %s\n\n":  "Library: %d files · %s\n\n",
	"Что сделать?":                      "What would you like to do?",
	"↓ Скачать видео":                   "↓ Download video",
	"♫ Скачать аудио":                   "♫ Download audio",
	"Настройки":                         "Settings",
	"Диагностика":                       "Diagnostics",
	"Выход":                             "Exit",
	"Enter — главное меню":              "Enter — main menu",
	"Новая загрузка":                    "New download",
	"Пустая строка — вернуться в меню.": "Leave the URL blank to return to the menu.",
	"Готово! Файлы доступны в разделе «Мои файлы».": "Done! Your files are available in My files.",
	"Качество":                       "Quality",
	"Выберите настройку":             "Select a setting",
	"Настройки сохранены.":           "Settings saved.",
	"Этот ролик уже есть в очереди.": "This video is already in the queue.",
	"Tab — переключение панелей. v — видео, a — аудио.": "Tab switches panels. v adds video; a adds audio.",
	"В очереди":                            "Queued",
	"Получение данных":                     "Fetching metadata",
	"Выберите качество: Enter":             "Choose quality: Enter",
	"Готов к загрузке":                     "Ready to download",
	"Скачивание":                           "Downloading",
	"Готово":                               "Done",
	"Ошибка":                               "Failed",
	"Отмена…":                              "Canceling…",
	"Отменено":                             "Canceled",
	"Повтор задания":                       "Retrying job",
	"Остановить задания и выйти?":          "Stop all jobs and exit?",
	"Продолжить работу":                    "Keep working",
	"Остановить и выйти":                   "Stop and exit",
	"Добавлено в очередь":                  "Added to queue",
	"Активно":                              "Active",
	"Увеличьте терминал: минимум 73 × 20.": "Resize the terminal to at least 73 × 20.",
	"Задания":                              "Jobs",
	"v — видео · a — аудио":                "v: add video · a: add audio",
	"Журнал событий":                       "Event log",
	"Tab: панель  ↑↓: выбор  Enter: открыть/качество  v/a: добавить  s: настройки":                  "Tab: focus  Up/Down: select  Enter: open/quality  v/a: add  s: settings",
	"x: отменить  r: повтор/обновить  o: Finder  f: папка  q: выход  Esc: назад":                    "x: cancel  r: retry/refresh  o: Finder  f: folder  q: quit  Esc: back",
	"Enter: подтвердить · Esc: назад. Загрузки продолжаются.":                                       "Enter: confirm · Esc: back. Downloads keep running.",
	"Linux: установите yt-dlp, FFmpeg/ffprobe и Deno; для открытия файлов — xdg-utils. См. README.": "Linux: install yt-dlp, FFmpeg/ffprobe and Deno; install xdg-utils to open files. See README.",
	"Отчёт для обращения об ошибке: macdown doctor --json":                                          "Shareable issue report: macdown doctor --json",
	"Проверка выявила проблемы. См. подсказки в отчёте.":                                            "Checks found problems. See the report for next steps.",
}
