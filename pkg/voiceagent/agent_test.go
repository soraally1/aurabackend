package voiceagent

import (
	"context"
	"testing"
)

func intPtr(v int) *int       { return &v }
func fltPtr(v float64) *float64 { return &v }

func TestFallbackGreetingAndMultiFieldExtraction(t *testing.T) {
	c := NewWithAPIKey("")

	// 1. Initial greeting asks for usia
	greet, err := c.ProcessTurn(context.Background(), ChatRequest{
		UserName: "Daffa",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if greet.IsComplete || len(greet.MissingFields) != 4 {
		t.Fatalf("expected 4 missing fields, got %+v", greet.MissingFields)
	}

	// 2. User answers multiple fields at once cleanly
	step2, err := c.ProcessTurn(context.Background(), ChatRequest{
		UserName:       "Daffa",
		UserMessage:    "Umurku 23 tahun, tinggi badan 172 cm, berat 66 kg",
		CurrentProfile: greet.UpdatedProfile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step2.UpdatedProfile.Usia == nil || *step2.UpdatedProfile.Usia != 23 {
		t.Fatalf("expected usia=23, got %+v", step2.UpdatedProfile.Usia)
	}
	if step2.UpdatedProfile.TinggiBadan == nil || *step2.UpdatedProfile.TinggiBadan != 172 {
		t.Fatalf("expected tinggi=172, got %+v", step2.UpdatedProfile.TinggiBadan)
	}
	if step2.UpdatedProfile.BeratBadan == nil || *step2.UpdatedProfile.BeratBadan != 66 {
		t.Fatalf("expected berat=66, got %+v", step2.UpdatedProfile.BeratBadan)
	}
	if len(step2.MissingFields) != 1 || step2.MissingFields[0] != "waktu_di_jalan" {
		t.Fatalf("expected only waktu_di_jalan missing, got %+v", step2.MissingFields)
	}

	// 3. User says WFH -> 0 hours -> profile complete
	step3, err := c.ProcessTurn(context.Background(), ChatRequest{
		UserName:       "Daffa",
		UserMessage:    "Aku kerja dari rumah WFH",
		CurrentProfile: step2.UpdatedProfile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !step3.IsComplete || step3.UpdatedProfile.WaktuDiJalan == nil || *step3.UpdatedProfile.WaktuDiJalan != 0 {
		t.Fatalf("expected complete profile with waktu=0, got %+v", step3)
	}

	// 4. User confirms submission
	step4, err := c.ProcessTurn(context.Background(), ChatRequest{
		UserName:       "Daffa",
		UserMessage:    "Ya sudah benar, simpan dan lanjutkan",
		CurrentProfile: step3.UpdatedProfile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !step4.ReadyToSubmit {
		t.Fatalf("expected ReadyToSubmit=true after confirmation, got %+v", step4)
	}
}

func TestMergeAndValidateBounds(t *testing.T) {
	curr := ProfileDraft{Usia: intPtr(20)}
	var out llmTurnOutput
	out.Extracted.Usia = fltPtr(999)      // invalid -> ignored, keeps 20
	out.Extracted.TinggiBadan = fltPtr(1.72) // meters -> 172 cm

	updated, filled := mergeAndValidateProfile(curr, out)
	if updated.Usia == nil || *updated.Usia != 20 {
		t.Fatalf("expected usia preserved at 20, got %+v", updated.Usia)
	}
	if updated.TinggiBadan == nil || *updated.TinggiBadan != 172 {
		t.Fatalf("expected meters 1.72 normalized to 172 cm, got %+v", updated.TinggiBadan)
	}
	if len(filled) != 1 || filled[0] != "tinggi_badan" {
		t.Fatalf("unexpected filled list: %+v", filled)
	}
}
