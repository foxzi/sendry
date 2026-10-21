package handlers

import (
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/stats"
)

// errorTypeCount is the number of failed and still retrying messages with one error type
type errorTypeCount struct {
	Type     stats.ErrorType
	Failed   int
	Retrying int
}

// summarizeErrors groups job item or send errors by type in display order, skipping types without items
func summarizeErrors(counts []models.ErrorCount) []errorTypeCount {
	byType := make(map[stats.ErrorType]*errorTypeCount)
	for _, c := range counts {
		et := stats.ClassifyError(c.Error)
		row := byType[et]
		if row == nil {
			row = &errorTypeCount{Type: et}
			byType[et] = row
		}
		switch c.Status {
		case "failed":
			row.Failed += c.Count
		case "queued":
			row.Retrying += c.Count
		}
	}

	var result []errorTypeCount
	for _, et := range stats.ErrorTypes {
		if row := byType[et]; row != nil && row.Failed+row.Retrying > 0 {
			result = append(result, *row)
		}
	}
	return result
}
