package terminal

import (
	"context"
	"github.com/bfcamel/CamelClip/internal/desktop/browsers"
)

func browserChoices(ctx context.Context, current string, first bool) ([]choice, string) {
	catalog := browsers.Discover(ctx)
	items := []choice{{tr("Не использовать"), ""}}
	found := current == ""
	for _, item := range catalog.Options {
		label := item.Label
		if item.Value == catalog.Suggested {
			label += " · " + tr("Системный")
		}
		items = append(items, choice{label, item.Value})
		if item.Value == current {
			found = true
		}
	}
	if !found {
		items = append(items, choice{current, current})
	}
	if first && current == "" {
		current = catalog.Suggested
	}
	return items, current
}
func (m *menu) pickBrowser(ctx context.Context, current string, first bool) (string, error) {
	items, preferred := browserChoices(ctx, current, first)
	return m.selectItem(ctx, tr("Браузер cookies"), items, preferred)
}
func (u *panelUI) pickBrowser(kind, current string) {
	items, preferred := browserChoices(u.ctx, current, false)
	u.showChoices(kind, tr("Браузер cookies"), items, preferred)
}
