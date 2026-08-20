package worker

import (
	"context"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
	"github.com/rs/zerolog"
)

// AlertNotifierWorker periodic ticker KHUSUS condition_type="threshold".
// new_issue TIDAK lewat sini — itu event-driven di ingest_consumer.go.
type AlertNotifierWorker struct {
	evaluateAlert	*usecase.EvaluateAlertUsecase
	logger			zerolog.Logger
	interval		time.Duration
}

func NewAlertNotifierWorker(evaluateAlert *usecase.EvaluateAlertUsecase, logger zerolog.Logger, interval time.Duration) *AlertNotifierWorker {
	return &AlertNotifierWorker{evaluateAlert: evaluateAlert, logger: logger, interval: interval}
}

func (w *AlertNotifierWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info().Dur("interval", w.interval).Msg("alert notifier worker started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.evaluateAlert.EvaluateThresholds(ctx); err != nil {
				w.logger.Error().Err(err).Msg("failed to evaluate threshold alert rules")
			}
		}
	}
}