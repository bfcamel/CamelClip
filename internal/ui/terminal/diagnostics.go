// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package terminal

import "strings"

// Structured reports use stable English messages; human reports follow the UI language.
func diagnosticText(message string) string {
	languageMu.RLock()
	defer languageMu.RUnlock()
	if currentLanguage != "ru" {
		return message
	}
	if translated, ok := diagnosticRussian[message]; ok {
		return translated
	}
	if strings.HasPrefix(message, "Install ") && strings.HasSuffix(message, " and run doctor again; see README for your OS.") {
		tool := strings.TrimSuffix(strings.TrimPrefix(message, "Install "), " and run doctor again; see README for your OS.")
		return "Установите " + tool + " и повторите диагностику. Инструкция для вашей ОС — в README."
	}
	return message
}

var diagnosticRussian = map[string]string{
	"Executable is missing from PATH.":                                           "Программа не найдена в PATH.",
	"Executable could not be started or timed out.":                              "Программу не удалось запустить или она не ответила вовремя.",
	"Reinstall the dependency and check execute permissions.":                    "Переустановите зависимость и проверьте разрешение на запуск.",
	"Selected encoder is available.":                                             "Выбранный кодек доступен.",
	"Selected encoder is absent from this FFmpeg build.":                         "В этой сборке FFmpeg нет выбранного кодека.",
	"Install a full FFmpeg build or choose another codec in Settings.":           "Установите полную сборку FFmpeg или выберите другой кодек в настройках.",
	"A path component is a file.":                                                "Вместо одного из каталогов указан файл.",
	"Choose an accessible directory in Settings.":                                "Выберите доступную папку в настройках.",
	"Directory cannot be accessed.":                                              "Не удалось получить доступ к папке.",
	"Check directory permissions in Settings.":                                   "Проверьте права доступа к папке, указанной в настройках.",
	"Directory is not writable.":                                                 "Нет разрешения на запись в папку.",
	"Check permissions or choose another directory.":                             "Проверьте разрешения или выберите другую папку.",
	"Write probe failed.":                                                        "Пробная запись завершилась ошибкой.",
	"Check storage availability.":                                                "Проверьте доступность диска.",
	"Less than 512 MiB of free space.":                                           "Свободно менее 512 МиБ.",
	"Free disk space; conversion temporarily needs space for source and result.": "Освободите место: при конвертации временно хранятся исходный и готовый файлы.",
	"Directory is writable.":                                                     "Запись в папку доступна.",
	"No saved settings; defaults are in use.":                                    "Сохранённых настроек нет; используются стандартные.",
	"Saved settings are unreadable or invalid; startup uses defaults.":           "Настройки повреждены или недоступны; при запуске используются стандартные.",
	"Back up settings.json and correct or rename it.":                            "Сделайте копию settings.json, затем исправьте или переименуйте файл.",
	"Saved settings are valid.":                                                  "Настройки корректны.",
	"Queue has not been created yet.":                                            "Очередь ещё не создана.",
	"Queue could not be read or failed its integrity check.":                     "Не удалось прочитать очередь или нарушена целостность базы.",
	"Close other sessions, back up queue.sqlite3 with its -wal/-shm files, then follow docs/troubleshooting.md. Do not delete the database.": "Закройте другие сеансы, скопируйте queue.sqlite3 вместе с файлами -wal/-shm и следуйте docs/troubleshooting.md. Не удаляйте базу.",
	"SQLite integrity check passed.":                        "Целостность SQLite подтверждена.",
	"Browser cookies are selected; contents were not read.": "Выбраны cookies браузера; их содержимое не читалось.",
	"Use the same Firefox/Chrome profile and network. Close the browser if its database is locked. Flatpak/Snap profiles may require an explicit profile path.": "Используйте тот же профиль Firefox/Chrome и ту же сеть. Если база занята, закройте браузер. Для Flatpak/Snap может понадобиться полный путь к профилю.",
	"Offline checks cannot verify YouTube authentication or JavaScript challenges.":                                                                             "Без обращения к сайту нельзя проверить вход в YouTube и JavaScript-проверки.",
	"Install yt-dlp EJS components and a supported JavaScript runtime; see README.":                                                                             "Установите компоненты EJS для yt-dlp и поддерживаемую среду JavaScript; см. README.",
	"Headless session: downloads work; opening files requires a desktop.":                                                                                       "Сеанс без рабочего стола: загрузки доступны, открытие файлов требует графического окружения.",
	"xdg-open is missing.":                              "Не найден xdg-open.",
	"Install xdg-utils to open files from the library.": "Установите xdg-utils для открытия файлов из библиотеки.",
	"Desktop opener is available.":                      "Открытие файлов через рабочий стол доступно.",
}
