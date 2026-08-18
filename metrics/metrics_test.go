package metrics

import (
	"testing"
	"time"
)

func TestRecordAPIUsage(t *testing.T) {
	InitDefaultCollector("")
	RecordAPIUsage("openai-compatible", APIUsage{
		PromptTokens:       100,
		CompletionTokens:   40,
		TotalTokens:        140,
		Duration:           2 * time.Second,
		InputCharacters:    400,
		OutputCharacters:   160,
		Requests:           2,
		ResponsesWithUsage: 1,
		Success:            true,
	})

	wants := map[string]float64{
		"ai_api_requests_total":          2,
		"ai_api_latest_duration_seconds": 2,
		"ai_api_latest_tokens_used":      140,
		"ai_api_prompt_tokens_total":     100,
		"ai_api_completion_tokens_total": 40,
		"ai_api_input_characters_total":  400,
		"ai_api_output_characters_total": 160,
		"ai_api_retries_total":           1,
		"ai_api_usage_responses_total":   1,
		"ai_api_usage_missing_total":     1,
		"ai_api_success_total":           1,
	}

	for name, want := range wants {
		got := defaultCollector.GetMetricsByName(name)
		if len(got) != 1 {
			t.Fatalf("%s metric count = %d, want 1", name, len(got))
		}
		if got[0].Value != want {
			t.Errorf("%s = %v, want %v", name, got[0].Value, want)
		}
	}
}

func TestMetricKeyIsStableAcrossLabelOrder(t *testing.T) {
	collector := NewMetricsCollector("")
	collector.RecordCounter("requests", 1, map[string]string{"provider": "openai", "success": "true"}, "")
	collector.RecordCounter("requests", 1, map[string]string{"success": "true", "provider": "openai"}, "")

	metrics := collector.GetMetricsByName("requests")
	if len(metrics) != 1 {
		t.Fatalf("metric count = %d, want 1", len(metrics))
	}
	if metrics[0].Value != 2 {
		t.Errorf("metric value = %v, want 2", metrics[0].Value)
	}
}

func TestMetricsReadsReturnDetachedSnapshots(t *testing.T) {
	collector := NewMetricsCollector("")
	collector.RecordCounter("requests", 1, map[string]string{"provider": "openai"}, "")

	first := collector.GetMetricsByName("requests")
	first[0].Value = 99
	first[0].Labels["provider"] = "changed"
	second := collector.GetMetricsByName("requests")
	if second[0].Value != 1 || second[0].Labels["provider"] != "openai" {
		t.Errorf("internal metric mutated through snapshot: %#v", second[0])
	}

	all := collector.GetAllMetrics()
	for _, metric := range all {
		metric.Value = 42
	}
	if got := collector.GetMetricsByName("requests")[0].Value; got != 1 {
		t.Errorf("internal metric value = %v after GetAllMetrics mutation, want 1", got)
	}
}
