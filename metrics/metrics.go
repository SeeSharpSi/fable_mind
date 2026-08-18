package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
	Summary   MetricType = "summary"
)

// Metric represents a single metric measurement
type Metric struct {
	Name        string            `json:"name"`
	Type        MetricType        `json:"type"`
	Value       float64           `json:"value"`
	Labels      map[string]string `json:"labels,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Description string            `json:"description,omitempty"`
}

// MetricsCollector collects and manages application metrics
type MetricsCollector struct {
	metrics     map[string]*Metric
	mutex       sync.RWMutex
	externalURL string
	client      *http.Client
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(externalURL string) *MetricsCollector {
	return &MetricsCollector{
		metrics:     make(map[string]*Metric),
		externalURL: externalURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// RecordCounter increments a counter metric
func (mc *MetricsCollector) RecordCounter(name string, value float64, labels map[string]string, description string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	key := mc.getMetricKey(name, labels)
	if metric, exists := mc.metrics[key]; exists {
		metric.Value += value
		metric.Timestamp = time.Now()
	} else {
		mc.metrics[key] = &Metric{
			Name:        name,
			Type:        Counter,
			Value:       value,
			Labels:      labels,
			Timestamp:   time.Now(),
			Description: description,
		}
	}

	// Send to external database if configured
	if mc.externalURL != "" {
		go mc.sendToExternal(cloneMetric(mc.metrics[key]))
	}
}

// SetGauge sets a gauge metric to a specific value
func (mc *MetricsCollector) SetGauge(name string, value float64, labels map[string]string, description string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	key := mc.getMetricKey(name, labels)
	mc.metrics[key] = &Metric{
		Name:        name,
		Type:        Gauge,
		Value:       value,
		Labels:      labels,
		Timestamp:   time.Now(),
		Description: description,
	}

	// Send to external database if configured
	if mc.externalURL != "" {
		go mc.sendToExternal(cloneMetric(mc.metrics[key]))
	}
}

// RecordHistogram records a histogram observation
func (mc *MetricsCollector) RecordHistogram(name string, value float64, labels map[string]string, description string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	key := mc.getMetricKey(name, labels)
	if metric, exists := mc.metrics[key]; exists {
		// For histogram, we could implement buckets, but for simplicity we'll just track the latest value
		metric.Value = value
		metric.Timestamp = time.Now()
	} else {
		mc.metrics[key] = &Metric{
			Name:        name,
			Type:        Histogram,
			Value:       value,
			Labels:      labels,
			Timestamp:   time.Now(),
			Description: description,
		}
	}

	// Send to external database if configured
	if mc.externalURL != "" {
		go mc.sendToExternal(cloneMetric(mc.metrics[key]))
	}
}

// GetAllMetrics returns all collected metrics
func (mc *MetricsCollector) GetAllMetrics() map[string]*Metric {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	result := make(map[string]*Metric)
	for k, v := range mc.metrics {
		metric := cloneMetric(v)
		result[k] = &metric
	}
	return result
}

// GetMetricsByName returns metrics filtered by name
func (mc *MetricsCollector) GetMetricsByName(name string) []*Metric {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	var result []*Metric
	for _, metric := range mc.metrics {
		if metric.Name == name {
			copy := cloneMetric(metric)
			result = append(result, &copy)
		}
	}
	return result
}

// getMetricKey generates a unique key for a metric based on name and labels
func (mc *MetricsCollector) getMetricKey(name string, labels map[string]string) string {
	key := name
	if labels != nil {
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := labels[k]
			key += fmt.Sprintf("{%s=%s}", k, v)
		}
	}
	return key
}

// sendToExternal sends a metric to an external database
func (mc *MetricsCollector) sendToExternal(metric Metric) {
	if mc.externalURL == "" {
		return
	}

	jsonData, err := json.Marshal(metric)
	if err != nil {
		// In a production system, you'd want to log this error
		return
	}

	req, err := http.NewRequest("POST", mc.externalURL+"/metrics", bytes.NewBuffer(jsonData))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := mc.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// In production, you might want to check resp.StatusCode and handle errors
}

func cloneMetric(metric *Metric) Metric {
	copy := *metric
	if metric.Labels != nil {
		copy.Labels = make(map[string]string, len(metric.Labels))
		for key, value := range metric.Labels {
			copy.Labels[key] = value
		}
	}
	return copy
}

// Global metrics collector instance
var defaultCollector *MetricsCollector

// InitDefaultCollector initializes the default metrics collector
func InitDefaultCollector(externalURL string) {
	defaultCollector = NewMetricsCollector(externalURL)
}

// RecordStoryGeneration records metrics for story generation
func RecordStoryGeneration(duration time.Duration, genre string, difficulty string, success bool) {
	if defaultCollector == nil {
		return
	}

	labels := map[string]string{
		"genre":      genre,
		"difficulty": difficulty,
		"success":    strconv.FormatBool(success),
	}

	defaultCollector.RecordCounter("story_generation_total", 1, labels, "Total number of story generation requests")
	defaultCollector.RecordHistogram("story_generation_duration", duration.Seconds(), labels, "Duration of story generation in seconds")

	if success {
		defaultCollector.RecordCounter("story_generation_success_total", 1, labels, "Total number of successful story generations")
	} else {
		defaultCollector.RecordCounter("story_generation_error_total", 1, labels, "Total number of failed story generations")
	}
}

// APIUsage describes aggregate model work for one story request.
type APIUsage struct {
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	Duration           time.Duration
	InputCharacters    int
	OutputCharacters   int
	Requests           int
	ResponsesWithUsage int
	Success            bool
}

// RecordAPIUsage records AI API usage metrics.
func RecordAPIUsage(provider string, usage APIUsage) {
	if defaultCollector == nil {
		return
	}

	labels := map[string]string{
		"provider": provider,
		"success":  strconv.FormatBool(usage.Success),
	}

	defaultCollector.RecordCounter("ai_api_requests_total", float64(usage.Requests), labels, "Total number of AI API requests")
	defaultCollector.SetGauge("ai_api_latest_duration_seconds", usage.Duration.Seconds(), labels, "Duration of the latest story request's AI calls")
	defaultCollector.RecordCounter("ai_api_prompt_tokens_total", float64(usage.PromptTokens), labels, "Prompt tokens reported by AI endpoints")
	defaultCollector.RecordCounter("ai_api_completion_tokens_total", float64(usage.CompletionTokens), labels, "Completion tokens reported by AI endpoints")
	defaultCollector.RecordCounter("ai_api_input_characters_total", float64(usage.InputCharacters), labels, "Input characters sent to AI endpoints")
	defaultCollector.RecordCounter("ai_api_output_characters_total", float64(usage.OutputCharacters), labels, "Output characters returned by AI endpoints")
	if retries := max(usage.Requests-1, 0); retries > 0 {
		defaultCollector.RecordCounter("ai_api_retries_total", float64(retries), labels, "AI retry requests")
	}
	if usage.ResponsesWithUsage > 0 {
		defaultCollector.RecordCounter("ai_api_usage_responses_total", float64(usage.ResponsesWithUsage), labels, "AI responses containing token usage")
	}
	if missingUsage := max(usage.Requests-usage.ResponsesWithUsage, 0); missingUsage > 0 {
		defaultCollector.RecordCounter("ai_api_usage_missing_total", float64(missingUsage), labels, "AI responses without token usage")
	}
	if usage.ResponsesWithUsage > 0 {
		defaultCollector.SetGauge("ai_api_latest_tokens_used", float64(usage.TotalTokens), labels, "Tokens reported for the latest story request")
	}

	if usage.Success {
		defaultCollector.RecordCounter("ai_api_success_total", 1, labels, "Total number of successful AI API calls")
	} else {
		defaultCollector.RecordCounter("ai_api_error_total", 1, labels, "Total number of failed AI API calls")
	}
}

// RecordUserActivity records user activity metrics.
func RecordUserActivity(action string, genre string, requestDuration time.Duration) {
	if defaultCollector == nil {
		return
	}

	labels := map[string]string{
		"action": action,
		"genre":  genre,
	}

	defaultCollector.RecordCounter("user_activity_total", 1, labels, "Total number of user actions")
	defaultCollector.SetGauge("user_latest_request_duration_seconds", requestDuration.Seconds(), labels, "Duration of latest user request in seconds")
}

// RecordError records application errors.
func RecordError(errorType string) {
	if defaultCollector == nil {
		return
	}

	labels := map[string]string{
		"error_type": errorType,
	}

	defaultCollector.RecordCounter("application_errors_total", 1, labels, "Total number of application errors")
}

// RecordRateLimit records rate limiting events
func RecordRateLimit(identifier string, blocked bool) {
	if defaultCollector == nil {
		return
	}

	labels := map[string]string{
		"identifier": identifier,
		"blocked":    strconv.FormatBool(blocked),
	}

	defaultCollector.RecordCounter("rate_limit_events_total", 1, labels, "Total number of rate limiting events")
}

// GetMetricsEndpoint returns an HTTP handler for the metrics endpoint
func GetMetricsEndpoint() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if defaultCollector == nil {
			http.Error(w, "Metrics collector not initialized", http.StatusInternalServerError)
			return
		}

		metrics := defaultCollector.GetAllMetrics()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
	}
}
