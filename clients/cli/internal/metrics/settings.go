package metrics

import (
	"time"

	"charm.land/huh/v2"
)

type settingsResult struct {
	panels   [4]bool
	interval time.Duration
}

func newSettingsForm(panels [4]bool, interval time.Duration) (*huh.Form, *[]int, *time.Duration) {
	var selectedPanels []int
	for i, v := range panels {
		if v {
			selectedPanels = append(selectedPanels, i)
		}
	}

	selectedInterval := interval

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Visible Panels").
				Options(
					huh.NewOption("LLM Stats", 0),
					huh.NewOption("Tool Stats", 1),
					huh.NewOption("Messages", 2),
					huh.NewOption("System", 3),
				).
				Value(&selectedPanels),
			huh.NewSelect[time.Duration]().
				Title("Refresh Interval").
				Options(
					huh.NewOption("1s", time.Second),
					huh.NewOption("2s", 2*time.Second),
					huh.NewOption("5s", 5*time.Second),
					huh.NewOption("10s", 10*time.Second),
				).
				Value(&selectedInterval),
		),
	)

	return form, &selectedPanels, &selectedInterval
}

func applySettings(selectedPanels []int, interval time.Duration) settingsResult {
	var panels [4]bool
	for _, idx := range selectedPanels {
		if idx >= 0 && idx < 4 {
			panels[idx] = true
		}
	}
	return settingsResult{panels: panels, interval: interval}
}
