package voiceagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ProfileDraft mirrors AURA's required registration profile fields.
type ProfileDraft struct {
	Usia         *int     `json:"usia"`
	TinggiBadan  *float64 `json:"tinggi_badan"`
	BeratBadan   *float64 `json:"berat_badan"`
	WaktuDiJalan *float64 `json:"waktu_di_jalan"`
}

// ChatMessage represents a single conversation turn between AURA AI and the user.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the payload for dynamic conversational form filling.
type ChatRequest struct {
	UserName       string        `json:"user_name"`
	UserMessage    string        `json:"user_message"`
	History        []ChatMessage `json:"history"`
	CurrentProfile ProfileDraft  `json:"current_profile"`
}

// ChatResponse returns the conversational reply, updated form fields, and completion state.
type ChatResponse struct {
	Transcript     string       `json:"transcript,omitempty"`
	AssistantReply string       `json:"assistant_reply"`
	UpdatedProfile ProfileDraft `json:"updated_profile"`
	NewlyFilled    []string     `json:"newly_filled"`
	MissingFields  []string     `json:"missing_fields"`
	IsComplete     bool         `json:"is_complete"`
	ReadyToSubmit  bool         `json:"ready_to_submit"`
	ModelUsed      string       `json:"model_used"`
}

// Client talks to Groq Whisper STT and Groq Chat using GROQ_AURA_AGENT.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewFromEnv creates a Client using GROQ_AURA_AGENT (falling back to GROQ_AURA2/GROQ_API_KEY).
func NewFromEnv() *Client {
	key := strings.TrimSpace(os.Getenv("GROQ_AURA_AGENT"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("GROQ_AURA2"))
	}
	if key == "" {
		key = strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	}
	return &Client{
		apiKey: key,
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

// NewWithAPIKey initializes a Client with an explicit key (used in tests).
func NewWithAPIKey(apiKey string) *Client {
	return &Client{
		apiKey: strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

// HasAPIKey reports whether an API key is configured.
func (c *Client) HasAPIKey() bool {
	return c != nil && c.apiKey != ""
}

// TranscribeAudio runs Groq Whisper Speech-to-Text in Indonesian.
func (c *Client) TranscribeAudio(ctx context.Context, audioBytes []byte, filename string) (string, error) {
	if !c.HasAPIKey() {
		return "", fmt.Errorf("GROQ_AURA_AGENT api key is not configured")
	}
	if len(audioBytes) < 32 {
		return "", fmt.Errorf("audio file is empty or too short")
	}
	if strings.TrimSpace(filename) == "" {
		filename = "speech.wav"
	}

	models := []string{"whisper-large-v3-turbo", "whisper-large-v3"}
	var lastErr error

	for _, model := range models {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			return "", err
		}
		if _, err := part.Write(audioBytes); err != nil {
			return "", err
		}
		_ = writer.WriteField("model", model)
		_ = writer.WriteField("language", "id")
		_ = writer.WriteField("temperature", "0")
		_ = writer.WriteField(
			"prompt",
			"Percakapan pengisian data usia, tinggi badan, berat badan, dan lama waktu di jalan dalam Bahasa Indonesia.",
		)
		_ = writer.WriteField("response_format", "json")
		if err := writer.Close(); err != nil {
			return "", err
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"https://api.groq.com/openai/v1/audio/transcriptions",
			&body,
		)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("[VoiceAgent STT] Groq request error (%s): %v", model, err)
			continue
		}
		respBytes, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("groq stt status %d: %s", resp.StatusCode, string(respBytes))
			log.Printf("[VoiceAgent STT] Non-200 (%s): %v", model, lastErr)
			continue
		}

		var parsed struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(respBytes, &parsed); err != nil {
			lastErr = err
			continue
		}

		return strings.TrimSpace(parsed.Text), nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("speech-to-text transcription failed")
	}
	return "", lastErr
}

type llmTurnOutput struct {
	AssistantReply  string `json:"assistant_reply"`
	Extracted       struct {
		Usia         *float64 `json:"usia"`
		TinggiBadan  *float64 `json:"tinggi_badan"`
		BeratBadan   *float64 `json:"berat_badan"`
		WaktuDiJalan *float64 `json:"waktu_di_jalan"`
	} `json:"extracted"`
	ConfirmedSubmit bool `json:"confirmed_submit"`
}

// ProcessTurn advances the conversational registration session dynamically.
func (c *Client) ProcessTurn(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// If no API key is present, fall back gracefully to local extraction.
	if !c.HasAPIKey() {
		return fallbackProcessTurn(req, "fallback-no-key"), nil
	}

	greetingName := strings.TrimSpace(req.UserName)
	if greetingName == "" {
		greetingName = "kamu"
	}

	systemPrompt := `Kamu adalah AURA, asisten pendaftaran suara berbahasa Indonesia yang ramah, hangat, dan sangat ringkas.
Tugasmu adalah mewawancarai pengguna secara alami seperti percakapan dua arah untuk melengkapi 4 field profil kesehatan:
1. usia (bilangan bulat 1 - 150 tahun)
2. tinggi_badan (angka 1 - 300 cm; jika user menyebut meter seperti "1 koma 7 meter" atau "170 senti", konversi ke cm misal 170)
3. berat_badan (angka 1 - 500 kg)
4. waktu_di_jalan (angka 0 - 24 jam per hari; jika user WFH / di rumah saja / tidak keluar, isi 0)

ATURAN PERCAKAPAN:
- Gunakan bahasa Indonesia yang natural dan sopan untuk dibacakan oleh Text-to-Speech (JANGAN gunakan markdown, tanda bintang *, pagar #, emoji berlebihan, atau bullet list).
- Maksimal 1-2 kalimat singkat yang jelas.
- Pengguna dapat menjawab lebih dari satu data sekaligus dalam satu kalimat. Ekstrak semua nilai yang valid sekaligus, lalu tanyakan secara dinamis HANYA field yang masih kosong.
- Jangan menghapus nilai current_profile yang sudah terisi kecuali pengguna secara eksplisit meralat/mengubah field tersebut.
- Jika seluruh 4 field sudah lengkap terisi: sebutkan ringkasan singkat keempat nilainya dan tanya apakah sudah sesuai untuk langsung disimpan masuk ke Dashboard.
- Set confirmed_submit = true HANYA jika seluruh 4 field sudah terisi lengkap dan pengguna menjawab setuju / konfirmasi (misalnya "ya", "lanjut", "simpan", "benar", "oke", "gas").

Kembalikan HANYA JSON valid dengan format:
{
  "assistant_reply": "<kalimat sapaan atau pertanyaan singkat selanjutnya>",
  "extracted": {
    "usia": <integer atau null>,
    "tinggi_badan": <number atau null>,
    "berat_badan": <number atau null>,
    "waktu_di_jalan": <number atau null>
  },
  "confirmed_submit": <boolean>
}`

	profileJSON, _ := json.Marshal(req.CurrentProfile)
	var contextBuilder strings.Builder
	contextBuilder.WriteString(fmt.Sprintf("Nama pengguna: %s\n", greetingName))
	contextBuilder.WriteString(fmt.Sprintf("Status form saat ini: %s\n", string(profileJSON)))
	if strings.TrimSpace(req.UserMessage) == "" {
		contextBuilder.WriteString("Ini adalah pembukaan percakapan. Sapa pengguna dengan namanya, jelaskan singkat bahwa kamu akan membantu mengisi data profil lewat obrolan suara, dan langsung tanyakan field pertama yang masih kosong.")
	} else {
		contextBuilder.WriteString(fmt.Sprintf("Ucapan pengguna terbaru (hasil Speech-to-Text): %q\nEkstrak angka/nilai relevan dan balas secara alami.", req.UserMessage))
	}

	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
	}
	// Keep up to last 8 conversation turns for crisp context
	startIdx := 0
	if len(req.History) > 8 {
		startIdx = len(req.History) - 8
	}
	for _, h := range req.History[startIdx:] {
		role := strings.ToLower(strings.TrimSpace(h.Role))
		if role != "assistant" && role != "user" {
			continue
		}
		if txt := strings.TrimSpace(h.Content); txt != "" {
			messages = append(messages, map[string]string{"role": role, "content": txt})
		}
	}
	messages = append(messages, map[string]string{"role": "user", "content": contextBuilder.String()})

	modelsToTry := []string{
		"openai/gpt-oss-120b",
		"openai/gpt-oss-20b",
		"llama-3.3-70b-versatile",
		"llama-3.1-8b-instant",
	}

	for _, model := range modelsToTry {
		payload := map[string]any{
			"model":           model,
			"messages":        messages,
			"response_format": map[string]string{"type": "json_object"},
			"temperature":     0.25,
		}
		rawBody, err := json.Marshal(payload)
		if err != nil {
			continue
		}

		httpReq, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"https://api.groq.com/openai/v1/chat/completions",
			bytes.NewReader(rawBody),
		)
		if err != nil {
			continue
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			log.Printf("[VoiceAgent LLM] HTTP error (%s): %v", model, err)
			continue
		}
		respBytes, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			log.Printf("[VoiceAgent LLM] Non-200 (%s, %d): %s", model, resp.StatusCode, string(respBytes))
			continue
		}

		var groqResp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(respBytes, &groqResp); err != nil || len(groqResp.Choices) == 0 {
			continue
		}

		var out llmTurnOutput
		content := strings.TrimSpace(groqResp.Choices[0].Message.Content)
		if err := json.Unmarshal([]byte(content), &out); err != nil {
			log.Printf("[VoiceAgent LLM] JSON parse error (%s): %v", model, err)
			continue
		}

		updated, newlyFilled := mergeAndValidateProfile(req.CurrentProfile, out)
		missing := computeMissingFields(updated)
		isComplete := len(missing) == 0
		readyToSubmit := isComplete && out.ConfirmedSubmit

		reply := strings.TrimSpace(out.AssistantReply)
		if reply == "" {
			reply = buildDefaultPrompt(req.UserName, updated, missing)
		}

		return ChatResponse{
			Transcript:     strings.TrimSpace(req.UserMessage),
			AssistantReply: reply,
			UpdatedProfile: updated,
			NewlyFilled:    newlyFilled,
			MissingFields:  missing,
			IsComplete:     isComplete,
			ReadyToSubmit:  readyToSubmit,
			ModelUsed:      model,
		}, nil
	}

	return fallbackProcessTurn(req, "fallback-rule"), nil
}

func mergeAndValidateProfile(curr ProfileDraft, out llmTurnOutput) (ProfileDraft, []string) {
	next := curr
	newlyFilled := make([]string, 0, 4)

	if out.Extracted.Usia != nil {
		u := int(math.Round(*out.Extracted.Usia))
		if u >= 1 && u <= 150 {
			if next.Usia == nil || *next.Usia != u {
				newlyFilled = append(newlyFilled, "usia")
			}
			next.Usia = &u
		}
	}

	if out.Extracted.TinggiBadan != nil {
		rawT := *out.Extracted.TinggiBadan
		if rawT > 0.5 && rawT <= 2.5 {
			// Normalize meter values like 1.72 -> 172 cm cleanly before rounding
			rawT = rawT * 100.0
		}
		t := math.Round(rawT*10) / 10
		if t >= 1 && t <= 300 {
			if next.TinggiBadan == nil || *next.TinggiBadan != t {
				newlyFilled = append(newlyFilled, "tinggi_badan")
			}
			next.TinggiBadan = &t
		}
	}

	if out.Extracted.BeratBadan != nil {
		b := math.Round(*out.Extracted.BeratBadan*10) / 10
		if b >= 1 && b <= 500 {
			if next.BeratBadan == nil || *next.BeratBadan != b {
				newlyFilled = append(newlyFilled, "berat_badan")
			}
			next.BeratBadan = &b
		}
	}

	if out.Extracted.WaktuDiJalan != nil {
		w := math.Round(*out.Extracted.WaktuDiJalan*10) / 10
		if w >= 0 && w <= 24 {
			if next.WaktuDiJalan == nil || *next.WaktuDiJalan != w {
				newlyFilled = append(newlyFilled, "waktu_di_jalan")
			}
			next.WaktuDiJalan = &w
		}
	}

	return next, newlyFilled
}

// MissingFields returns ordered slice of remaining required fields.
func MissingFields(p ProfileDraft) []string {
	return computeMissingFields(p)
}

func computeMissingFields(p ProfileDraft) []string {
	missing := make([]string, 0, 4)
	if p.Usia == nil || *p.Usia < 1 || *p.Usia > 150 {
		missing = append(missing, "usia")
	}
	if p.TinggiBadan == nil || *p.TinggiBadan < 1 || *p.TinggiBadan > 300 {
		missing = append(missing, "tinggi_badan")
	}
	if p.BeratBadan == nil || *p.BeratBadan < 1 || *p.BeratBadan > 500 {
		missing = append(missing, "berat_badan")
	}
	if p.WaktuDiJalan == nil || *p.WaktuDiJalan < 0 || *p.WaktuDiJalan > 24 {
		missing = append(missing, "waktu_di_jalan")
	}
	return missing
}

func buildDefaultPrompt(userName string, p ProfileDraft, missing []string) string {
	namePrefix := ""
	if trimmed := strings.TrimSpace(userName); trimmed != "" {
		namePrefix = trimmed + ", "
	}
	if len(missing) == 0 {
		return fmt.Sprintf(
			"Lengkap! Usia %d tahun, tinggi %.0f sentimeter, berat %.0f kilogram, dan waktu di jalan %.1f jam per hari. Sudah benar untuk langsung simpan ke Dashboard?",
			*p.Usia, *p.TinggiBadan, *p.BeratBadan, *p.WaktuDiJalan,
		)
	}
	switch missing[0] {
	case "usia":
		if p.Usia == nil && p.TinggiBadan == nil && p.BeratBadan == nil && p.WaktuDiJalan == nil {
			return fmt.Sprintf("Halo %saku AURA yang bakal bantu lengkapi profilmu lewat suara. Sekarang usiamu berapa tahun?", namePrefix)
		}
		return "Sekarang berapa usia kamu dalam tahun?"
	case "tinggi_badan":
		return "Oke tercatat! Tinggi badanmu kira-kira berapa sentimeter?"
	case "berat_badan":
		return "Sip, lanjut ya. Berat badanmu saat ini berapa kilogram?"
	case "waktu_di_jalan":
		return "Terakhir, rata-rata berapa jam sehari kamu berada di jalanan? Bilang saja nol jika di rumah."
	default:
		return "Bisa sebutkan detail data profilmu lagi?"
	}
}

var (
	reUsia   = regexp.MustCompile(`(?i)(?:umur|usia|umurnya|usianya)?\s*(\d{1,3})\s*(?:tahun|thn|yo)`)
	reTinggi = regexp.MustCompile(`(?i)(?:tinggi|tb|tingginya)?\s*(\d{2,3}(?:[.,]\d+)?)\s*(?:cm|senti|sentimeter)`)
	reBerat  = regexp.MustCompile(`(?i)(?:berat|bb|beratnya)?\s*(\d{2,3}(?:[.,]\d+)?)\s*(?:kg|kilo|kilogram)`)
	reWaktu  = regexp.MustCompile(`(?i)(?:waktu|jalan|dijalan|perjalanan|keluar|durasi)?\s*(\d{1,2}(?:[.,]\d+)?)\s*(?:jam)`)
	reSingle = regexp.MustCompile(`(\d+(?:[.,]\d+)?)`)
)

func fallbackProcessTurn(req ChatRequest, modelTag string) ChatResponse {
	msg := strings.TrimSpace(req.UserMessage)
	lower := strings.ToLower(msg)
	var out llmTurnOutput

	if m := reUsia.FindStringSubmatch(msg); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			out.Extracted.Usia = &v
		}
	}
	if m := reTinggi.FindStringSubmatch(msg); len(m) == 2 {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
			out.Extracted.TinggiBadan = &v
		}
	}
	if m := reBerat.FindStringSubmatch(msg); len(m) == 2 {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
			out.Extracted.BeratBadan = &v
		}
	}
	if m := reWaktu.FindStringSubmatch(msg); len(m) == 2 {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
			out.Extracted.WaktuDiJalan = &v
		}
	}

	missingBefore := computeMissingFields(req.CurrentProfile)
	if len(missingBefore) > 0 && msg != "" {
		// Handle WFH / zero outdoor text
		if missingBefore[0] == "waktu_di_jalan" && (strings.Contains(lower, "wfh") || strings.Contains(lower, "rumah") || strings.Contains(lower, "nol")) {
			zero := 0.0
			out.Extracted.WaktuDiJalan = &zero
		} else if out.Extracted.Usia == nil && out.Extracted.TinggiBadan == nil && out.Extracted.BeratBadan == nil && out.Extracted.WaktuDiJalan == nil {
			// Single plain number answer maps to current asked missing field
			if m := reSingle.FindStringSubmatch(msg); len(m) == 2 {
				if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
					switch missingBefore[0] {
					case "usia":
						out.Extracted.Usia = &v
					case "tinggi_badan":
						out.Extracted.TinggiBadan = &v
					case "berat_badan":
						out.Extracted.BeratBadan = &v
					case "waktu_di_jalan":
						out.Extracted.WaktuDiJalan = &v
					}
				}
			}
		}
	}

	if len(missingBefore) == 0 && (strings.Contains(lower, "ya") || strings.Contains(lower, "oke") || strings.Contains(lower, "simpan") || strings.Contains(lower, "lanjut") || strings.Contains(lower, "benar")) {
		out.ConfirmedSubmit = true
	}

	updated, newlyFilled := mergeAndValidateProfile(req.CurrentProfile, out)
	missing := computeMissingFields(updated)
	isComplete := len(missing) == 0

	return ChatResponse{
		Transcript:     msg,
		AssistantReply: buildDefaultPrompt(req.UserName, updated, missing),
		UpdatedProfile: updated,
		NewlyFilled:    newlyFilled,
		MissingFields:  missing,
		IsComplete:     isComplete,
		ReadyToSubmit:  isComplete && out.ConfirmedSubmit,
		ModelUsed:      modelTag,
	}
}
