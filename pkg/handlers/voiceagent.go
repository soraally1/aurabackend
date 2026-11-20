package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"aurabackend/pkg/httpx"
	"aurabackend/pkg/voiceagent"
)

const maxAgentAudioBytes = 10 << 20 // 10 MiB

// AgentTranscribe converts voice audio to text using Groq Whisper (GROQ_AURA_AGENT).
//
// POST /api/agent/transcribe (multipart/form-data: file)
func AgentTranscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentAudioBytes)
	if err := r.ParseMultipartForm(maxAgentAudioBytes); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_MULTIPART", "Payload audio tidak valid atau melebihi batas 10 MB.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "MISSING_AUDIO", "Field audio 'file' wajib dikirim.")
		return
	}
	defer func() { _ = file.Close() }()

	audioBytes, err := io.ReadAll(file)
	if err != nil || len(audioBytes) < 32 {
		httpx.Error(w, http.StatusBadRequest, "EMPTY_AUDIO", "File suara kosong atau rusak.")
		return
	}

	filename := "speech.wav"
	if header != nil && header.Filename != "" {
		filename = header.Filename
	}

	client := voiceagent.NewFromEnv()
	text, err := client.TranscribeAudio(r.Context(), audioBytes, filename)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "STT_FAILED", err.Error())
		return
	}

	httpx.OK(w, http.StatusOK, map[string]any{
		"text": text,
	})
}

// AgentRegistrationChat runs one dynamic text/state turn for registration profile conversation.
//
// POST /api/agent/registration-chat
func AgentRegistrationChat(w http.ResponseWriter, r *http.Request) {
	var req voiceagent.ChatRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	client := voiceagent.NewFromEnv()
	res, err := client.ProcessTurn(r.Context(), req)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "AGENT_FAILED", err.Error())
		return
	}

	httpx.OK(w, http.StatusOK, res)
}

// AgentVoiceTurn performs Speech-to-Text (Groq Whisper) + Dynamic Form Extraction (Groq LLM)
// in a single low-latency round-trip using GROQ_AURA_AGENT.
//
// POST /api/agent/voice-turn
func AgentVoiceTurn(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentAudioBytes)
	if err := r.ParseMultipartForm(maxAgentAudioBytes); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_MULTIPART", "Payload suara tidak valid atau melebihi batas 10 MB.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "MISSING_AUDIO", "Field audio 'file' wajib dikirim.")
		return
	}
	defer func() { _ = file.Close() }()

	audioBytes, err := io.ReadAll(file)
	if err != nil || len(audioBytes) < 32 {
		httpx.Error(w, http.StatusBadRequest, "EMPTY_AUDIO", "File rekaman suara terlalu pendek atau kosong.")
		return
	}

	filename := "speech.wav"
	if header != nil && header.Filename != "" {
		filename = header.Filename
	}

	var currentProfile voiceagent.ProfileDraft
	if rawProf := strings.TrimSpace(r.FormValue("current_profile")); rawProf != "" {
		_ = json.Unmarshal([]byte(rawProf), &currentProfile)
	}

	var history []voiceagent.ChatMessage
	if rawHist := strings.TrimSpace(r.FormValue("history")); rawHist != "" {
		_ = json.Unmarshal([]byte(rawHist), &history)
	}

	userName := strings.TrimSpace(r.FormValue("user_name"))

	client := voiceagent.NewFromEnv()
	transcript, err := client.TranscribeAudio(r.Context(), audioBytes, filename)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "STT_FAILED", err.Error())
		return
	}

	if strings.TrimSpace(transcript) == "" {
		missing := voiceagent.MissingFields(currentProfile)
		httpx.OK(w, http.StatusOK, voiceagent.ChatResponse{
			Transcript:     "",
			AssistantReply: "Maaf, suara kamu belum jelas tertangkap. Bisa tolong diucapkan sekali lagi lebih dekat ke mikrofon?",
			UpdatedProfile: currentProfile,
			NewlyFilled:    []string{},
			MissingFields:  missing,
			IsComplete:     len(missing) == 0,
			ReadyToSubmit:  false,
			ModelUsed:      "whisper-empty-retry",
		})
		return
	}

	res, err := client.ProcessTurn(r.Context(), voiceagent.ChatRequest{
		UserName:       userName,
		UserMessage:    transcript,
		History:        history,
		CurrentProfile: currentProfile,
	})
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "AGENT_FAILED", err.Error())
		return
	}

	httpx.OK(w, http.StatusOK, res)
}
