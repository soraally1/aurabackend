package mobility

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

// GroqClient calls Groq's OpenAI-compatible completions API using Llama-3.3-70b-versatile.
type GroqClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewGroqClient initializes Groq client.
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
	Model          string          `json:"model"`
	Messages       []groqMessage   `json:"messages"`
	ResponseFormat map[string]any `json:"response_format"`
	Temperature    float64         `json:"temperature"`
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

// AnalyzeExposureWithGroq sends the aggregated daily stats to Groq's LLM model.
func (g *GroqClient) AnalyzeExposureWithGroq(ctx context.Context, stats ExposureStats) (AIDiagnosis, error) {
	if g.apiKey == "" {
		return g.FallbackRuleBasedDiagnosis(stats), nil
	}

	systemPrompt := `Anda adalah dokter spesialis paru (pulmonologi) dan kedokteran okupasi dari platform AURA Health.
Tugas Anda adalah mengevaluasi data mobilitas harian dan paparan polusi udara pengguna, lalu memberikan diagnosa medis ramah, terpercaya, dan solutif.
Kembalikan HANYA format JSON valid tanpa tanda backtick atau markdown pembungkus. Format JSON yang wajib dipenuhi:
{
  "impact_score": <number float 0-100>,
  "risk_level": "<Rendah|Sedang|Tinggi|Berbahaya>",
  "summary": "<1-2 kalimat ringkasan kondisi pernapasan hari ini>",
  "doctor_diagnosis": "<penjelasan diagnosa klinis dari dokter paru tentang dampak partikulat PM2.5/AQI pada saluran pernapasan>",
  "recommendations": [
    "<langkah pencegahan medis konkret 1>",
    "<langkah pencegahan medis konkret 2>",
    "<langkah pencegahan medis konkret 3>",
    "<langkah pencegahan medis konkret 4>"
  ]
}`

	userPrompt := fmt.Sprintf(`Berikut data mobilitas dan polusi harian pengguna di Kota Semarang:
- Total Waktu Luar Ruangan (Outdoor/Jalan): %.1f menit (%.2f jam)
- Total Waktu Dalam Ruangan (Indoor/Stay): %.1f menit (%.2f jam)
- Rata-rata AQI Outdoor: %.1f
- Rata-rata PM2.5 Outdoor: %.1f µg/m³
- Area dengan Polusi Terburuk yang Dilewati: %s (AQI: %d)
- Jumlah Titik Waypoint Dianalisis: %d

Berikan skor dampak polusi 0-100, level risiko, penjelasan klinis dari dokter paru, serta 3-4 rekomendasi pencegahan praktis.`,
		stats.TotalOutdoorMinutes, stats.TotalOutdoorMinutes/60.0,
		stats.TotalIndoorMinutes, stats.TotalIndoorMinutes/60.0,
		stats.AverageOutdoorAQI,
		stats.AverageOutdoorPM25,
		stats.WorstAreaName, stats.WorstAreaAQI,
		stats.WaypointsProcessed,
	)

	reqPayload := groqRequest{
		Model: g.model,
		Messages: []groqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: map[string]any{"type": "json_object"},
		Temperature:    0.2,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return g.FallbackRuleBasedDiagnosis(stats), err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return g.FallbackRuleBasedDiagnosis(stats), err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		log.Printf("[Groq API] HTTP request error: %v, using clinical fallback", err)
		return g.FallbackRuleBasedDiagnosis(stats), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return g.FallbackRuleBasedDiagnosis(stats), nil
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Groq API] Non-200 response (%d): %s, using clinical fallback", resp.StatusCode, string(respBody))
		return g.FallbackRuleBasedDiagnosis(stats), nil
	}

	var gResp groqResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil || len(gResp.Choices) == 0 {
		return g.FallbackRuleBasedDiagnosis(stats), nil
	}

	var diagnosis AIDiagnosis
	content := strings.TrimSpace(gResp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &diagnosis); err != nil {
		log.Printf("[Groq API] Unmarshal JSON error: %v, using clinical fallback", err)
		return g.FallbackRuleBasedDiagnosis(stats), nil
	}

	return diagnosis, nil
}

// FallbackRuleBasedDiagnosis provides a high-fidelity clinical assessment if Groq API is unavailable.
func (g *GroqClient) FallbackRuleBasedDiagnosis(stats ExposureStats) AIDiagnosis {
	// Base calculations
	// 1. Time outdoor factor (up to 40 pts for 120 mins)
	timeFactor := math.Min(40.0, (stats.TotalOutdoorMinutes/120.0)*40.0)
	// 2. AQI factor (up to 50 pts for AQI 200)
	aqiFactor := math.Min(50.0, (stats.AverageOutdoorAQI/200.0)*50.0)
	// 3. Peak hotspot factor (up to 10 pts)
	peakFactor := 0.0
	if stats.WorstAreaAQI > 100 {
		peakFactor = math.Min(10.0, float64(stats.WorstAreaAQI-100)*0.1)
	}

	rawScore := math.Min(100.0, math.Max(10.0, timeFactor+aqiFactor+peakFactor))
	impactScore := math.Round(rawScore*10) / 10

	var riskLevel string
	var summary string
	var docDiagnosis string
	recommendations := make([]string, 0)

	worstArea := stats.WorstAreaName
	if worstArea == "" {
		worstArea = "jalur lalu lintas utama"
	}

	if impactScore < 38.0 {
		riskLevel = "Rendah"
		summary = fmt.Sprintf("Paparan polusi Anda hari ini tergolong rendah (durasi luar ruangan %.0f menit). Saluran pernapasan Anda dalam kondisi aman.", stats.TotalOutdoorMinutes)
		docDiagnosis = fmt.Sprintf("Kondisi paparan harian Anda terpantau baik dengan durasi outdoor terkontrol dan rata-rata AQI %.0f. Risiko iritasi mukosa atau spasme bronkus minimal. Pertahankan kebiasaan mobilitas hemat paparan ini.", stats.AverageOutdoorAQI)
		recommendations = append(recommendations,
			"Pertahankan hidrasi tubuh dengan minum air putih minimal 2 liter per hari.",
			"Lakukan ventilasi ruangan saat pagi hari ketika kualitas udara luar masih segar.",
			"Gunakan masker kain atau medis ringan jika berkendara sepeda motor santai.",
		)
	} else if impactScore < 68.0 {
		riskLevel = "Sedang"
		summary = fmt.Sprintf("Paparan polusi Anda hari ini berada pada tingkat sedang setelah beraktivitas luar ruangan selama %.0f menit di sekitar %s.", stats.TotalOutdoorMinutes, worstArea)
		docDiagnosis = fmt.Sprintf("Terdeteksi akumulasi partikulat halus (PM2.5 rata-rata %.1f µg/m³) akibat perjalanan luar ruangan. Paparan ini dapat memicu reaksi inflamasi ringan pada saluran pernapasan atas, tenggorokan kering, atau hidung tersumbat, terutama bagi pengidap alergi udara.", stats.AverageOutdoorPM25)
		recommendations = append(recommendations,
			"Gunakan masker standar medis atau KF94 saat bepergian melintasi jalan padat kendaraan.",
			"Bilas wajah dan bersihkan rongga hidung dengan air bersih atau larutan saline setelah sampai di rumah.",
			"Konsumsi buah bervitamin C tinggi (jeruk, jambu biji) untuk menangkal efek stres oksidatif partikulat polusi.",
			"Nyalakan ventilasi mekanis atau air purifier di kamar tidur.",
		)
	} else {
		riskLevel = "Tinggi"
		summary = fmt.Sprintf("Perhatian: Paparan polusi harian Anda tinggi dengan skor %.1f akibat durasi outdoor signifikan di area padat %s (AQI puncak %d).", impactScore, worstArea, stats.WorstAreaAQI)
		docDiagnosis = fmt.Sprintf("Paparan PM2.5 dan gas buang kendaraan dalam durasi %.0f menit berpotensi menyebabkan iritasi trakeobronkial akut, penurunan kapasitas vital paru sementara, serta peningkatan risiko batuk kronis. Sangat disarankan untuk membatasi aktivitas luar ruangan yang tidak mendesak.", stats.TotalOutdoorMinutes)
		recommendations = append(recommendations,
			"Wajib kenakan masker respirator N95/KF94 terpasang rapat (fit test) bila harus berada di luar ruangan.",
			"Gunakan obat semprot hidung saline (NaCl fisiologis) untuk membilas partikulat debu yang menempel di silia hidung.",
			"Konsumsi air hangat dan madu guna meredakan inflamasi dinding faring.",
			"Tutup rapat jendela dan pintu ruangan serta pasang pemurni udara (air purifier) berefisiensi HEPA.",
			"Jika mengalami batuk terus-menerus atau sesak napas, segera periksakan diri ke IGD rumah sakit terdekat.",
		)
	}

	return AIDiagnosis{
		ImpactScore:     impactScore,
		RiskLevel:       riskLevel,
		Summary:         summary,
		DoctorDiagnosis: docDiagnosis,
		Recommendations: recommendations,
	}
}
