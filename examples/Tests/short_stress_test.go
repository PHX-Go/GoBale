package tests

import (
	"fmt"
	"testing"

	"github.com/PHX-Go/GoBale"
	"github.com/PHX-Go/GoBale/models"
)

func BenchmarkGoBaleShortStress(b *testing.B) {
	bot := gobale.NewBot("111111:invalid_token_for_instant_circuit_breaker_trip", 0)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c := &gobale.Context{
				Bot: bot,
				Update: &models.Update{
					Message: &models.Message{
						Chat: models.Chat{
							ID: 999999999,
						},
					},
				},
			}

			var items []models.InlineKeyboardButton
			for i := 0; i < 15; i++ {
				items = append(items, models.NewInlineKeyboardButtonData(
					fmt.Sprintf("Item %d", i),
					fmt.Sprintf("cb_data_%d", i),
				))
			}

			_, _ = c.SendAutoPaginated("Stress Test", items, 4, gobale.WithMarkdown())
		}
	})
}