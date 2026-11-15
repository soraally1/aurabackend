package tracking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// GroqClient calls Groq's OpenAI-compatible completions API for session analysis.
type GroqClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewGroqClient initializes Groq client for tracking analysis.
func NewGroqClient(apiKey string) *GroqClient {
	return &GroqClient{
		apiKey: apiKey,
		model:  "llama-3.3-70b-versatile",
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqRequest struct {
	Model          string         `json:"model"`
	Messages       []groqMessage  `json:"messages"`
	ResponseFormat map[string]any `json:"response_format"`
	Temperature    float64        `json:"temperature"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// AnalyzeExposureWithGroq sends the aggregated session stats to Groq's LLM model.
func (g *GroqClient) AnalyzeExposureWithGroq(ctx context.Context, stats SessionStats) (HealthAnalysis, error) {
	return g.AnalyzeExposureWithUserGroq(ctx, stats, nil)
}

// AnalyzeExposureWithUserGroq sends aggregated session stats and optional user profile context to Groq.
func (g *GroqClient) AnalyzeExposureWithUserGroq(ctx context.Context, stats SessionStats, userCtx *UserProfileContext) (HealthAnalysis, error) {
	if g.apiKey == "" {
		return generateHealthAnalysisRuleBased(stats), nil
	}

	systemPrompt := `Anda adalah dokter spesialis paru (pulmonologi) dan kedokteran okupasi dari platform AURA Health.
Tugas Anda adalah mengevaluasi data mobilitas dan paparan polusi udara pengguna, lalu memberikan ringkasan dampak kesehatan serta saran perawatan pasca perjalanan yang ramah dan terpercaya.
Kembalikan HANYA format JSON valid tanpa tanda backtick atau markdown pembungkus. Format JSON yang wajib dipenuhi:
{
  "impact_score": <number float 0-100>,
  "risk_level": "<Rendah|Sedang|Tinggi|Berbahaya>",
  "summary": "<1-2 kalimat ringkasan kondisi pernapasan hari ini dalam Bahasa Indonesia>",
  "recommendations": [
    "<langkah perawatan pasca perjalanan 1>",
    "<langkah perawatan pasca perjalanan 2>",
    "<langkah perawatan pasca perjalanan 3>",
    "<langkah perawatan pasca perjalanan 4>"
  ]
}`

	profileInfo := ""
	if userCtx != nil && (userCtx.Usia > 0 || userCtx.WaktuDiJalan > 0) {
		profileInfo = fmt.Sprintf("\n- Profil Pengguna: Usia %d tahun, rata-rata waktu di jalan %.1f jam/hari", userCtx.Usia, userCtx.WaktuDiJalan)
	}

	userPrompt := fmt.Sprintf(`Berikut data perjalanan dan paparan polusi pengguna di Kota Semarang:
- Total Waktu Luar Ruangan (Outdoor/Jalan): %.1f menit (%.2f jam)
- Total Waktu Dalam Ruangan (Indoor/Stay): %.1f menit (%.2f jam)
- Rata-rata AQI Outdoor: %.1f
- Rata-rata PM2.5 Outdoor: %.1f µg/m³
- AQI Maksimum yang Dilewati: %d
- Area dengan Polusi Terburuk yang Dilewati: %s (AQI: %d)
- Jumlah Titik Waypoint Dianalisis: %d
- Jarak Total Perjalanan: %.2f km
- Durasi Total: %.1f menit%s

Berikan skor dampak polusi 0-100, level risiko, penjelasan ringkas, serta 3-4 tips perawatan pasca perjalanan praktis.`,
		stats.TotalOutdoorMinutes, stats.TotalOutdoorMinutes/60.0,
		stats.TotalIndoorMinutes, stats.TotalIndoorMinutes/60.0,
		stats.AverageOutdoorAQI,
		stats.AverageOutdoorPM25,
		stats.MaxAQI,
		stats.WorstAreaName, stats.WorstAreaAQI,
		stats.WaypointsProcessed,
		stats.DistanceKm,
		stats.DurationMinutes,
		profileInfo,
	)

	modelsToTry := []string{
		g.model,
		"openai/gpt-oss-120b",
		"openai/gpt-oss-20b",
		"llama-3.1-8b-instant",
	}

	for _, modelName := range modelsToTry {
		analysis, err := g.callGroqModel(ctx, modelName, systemPrompt, userPrompt)
		if err == nil && analysis.ImpactScore > 0 {
			return analysis, nil
		}
		if err != nil {
			log.Printf("[Groq] Model %s error: %v", modelName, err)
		}
	}

	log.Printf("[Groq] All models exhausted, using rule-based analysis")
	return generateHealthAnalysisRuleBased(stats), nil
}

func (g *GroqClient) callGroqModel(ctx context.Context, modelName, systemPrompt, userPrompt string) (HealthAnalysis, error) {
	reqPayload := groqRequest{
		Model: modelName,
		Messages: []groqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: map[string]any{"type": "json_object"},
		Temperature:    0.2,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return HealthAnalysis{}, fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return HealthAnalysis{}, fmt.Errorf("request creation error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AuraBackend-Tracking/1.0 (team@aura.app)")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return HealthAnalysis{}, fmt.Errorf("http error: %w", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return HealthAnalysis{}, fmt.Errorf("read error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return HealthAnalysis{}, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var gResp groqResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil || len(gResp.Choices) == 0 {
		return HealthAnalysis{}, fmt.Errorf("parse error: %w", err)
	}

	var diagnosis HealthAnalysis
	content := stripMarkdownCodeFences(strings.TrimSpace(gResp.Choices[0].Message.Content))
	if err := json.Unmarshal([]byte(content), &diagnosis); err != nil {
		return HealthAnalysis{}, fmt.Errorf("json unmarshal error: %w", err)
	}

	// Validate response
	if diagnosis.ImpactScore <= 0 || diagnosis.ImpactScore > 100 {
		diagnosis.ImpactScore = generateHealthAnalysisRuleBased(statsFallbackFromScore(diagnosis.ImpactScore)).ImpactScore
	}

	diagnosis.RiskLevel = normalizeRiskLevel(diagnosis.RiskLevel, diagnosis.ImpactScore)

	if diagnosis.Summary == "" {
		diagnosis.Summary = "Analisis polusi hari ini menunjukkan paparan moderat."
	}

	if len(diagnosis.Recommendations) == 0 {
		diagnosis.Recommendations = []string{
			"Bersihkan wajah dengan air mengalir dan pembersih lembut",
			"Minum air putih minimal 300ml untuk hidrasi",
			"Gunakan masker saat beraktivitas di luar ruangan",
		}
	}

	return diagnosis, nil
}

func stripMarkdownCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		if end := strings.LastIndex(s, "```"); end != -1 {
			s = s[:end]
		}
	}
	return strings.TrimSpace(s)
}

func normalizeRiskLevel(raw string, score float64) string {
	switch strings.TrimSpace(raw) {
	case "Rendah", "Sedang", "Tinggi", "Berbahaya":
		return strings.TrimSpace(raw)
	default:
		if score < 38.0 {
			return "Rendah"
		}
		if score < 68.0 {
			return "Sedang"
		}
		if score < 88.0 {
			return "Tinggi"
		}
		return "Berbahaya"
	}
}

func statsFallbackFromScore(score float64) SessionStats {
	if score > 100 {
		return SessionStats{TotalOutdoorMinutes: 120, AverageOutdoorAQI: 180, MaxAQI: 180}
	}
	return SessionStats{TotalOutdoorMinutes: 0, AverageOutdoorAQI: 0, MaxAQI: 0}
}
