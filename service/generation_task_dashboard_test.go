package service

import (
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestDashboardHourFollowsBeijingTime(t *testing.T) {
	location := repository.DashboardLocation()
	today := time.Date(2026, 11, 11, 0, 0, 0, 0, location)
	created := time.Date(2026, 11, 11, 11, 40, 0, 0, time.UTC).Format(timestampLayout)
	rows := []repository.DashboardTaskRow{{
		CreatedAt: created,
		Status:    model.GenerationTaskStatusSuccess,
		Modality:  "image",
		Model:     "gpt-image-2.6",
		Credits:   1,
	}}

	hourly, _, _, _, _, _ := aggregateDashboardSeries(rows, today, 1)
	if hourly[11].Tasks != 0 || hourly[19].Tasks != 1 {
		t.Fatalf("11:40 UTC should land at 19:00 Beijing, got hour 11 = %d and hour 19 = %d", hourly[11].Tasks, hourly[19].Tasks)
	}
}
