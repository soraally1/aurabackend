package hospitals

import (
	"math"
	"testing"
)

func TestIsValidHospital(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid hospitals
		{"RSUP Dr. Kariadi", "RSUP Dr. Kariadi", true},
		{"SMC RS Telogorejo", "SMC RS Telogorejo", true},
		{"RSUD Tugurejo", "RSUD Tugurejo", true},
		{"RS Columbia Asia Semarang", "RS Columbia Asia Semarang", true},
		{"RS Roemani Muhammadiyah", "RS Roemani Muhammadiyah", true},
		{"Rumah Sakit St Elisabeth", "Rumah Sakit St Elisabeth", true},
		{"RS Islam Sultan Agung", "RS Islam Sultan Agung", true},
		{"RS Bhayangkara Semarang", "RS Bhayangkara Semarang", true},
		{"RS Siloam", "RS Siloam", true},
		{"RSU William Booth", "RSU William Booth", true},

		// Blacklisted non-hospitals
		{"Puskesmas Halmahera", "Puskesmas Halmahera", false},
		{"PKM Pandanaran", "PKM Pandanaran", false},
		{"Klinik Pratama Sehat Selalu", "Klinik Pratama Sehat Selalu", false},
		{"Kimia Farma Clinic", "Kimia Farma Clinic", false},
		{"Apotek K-24 Pandanaran", "Apotek K-24 Pandanaran", false},
		{"Pharmacy Viva Generik", "Pharmacy Viva Generik", false},
		{"Laboratorium Klinik Cito", "Laboratorium Klinik Cito", false},
		{"Prodia Lab Semarang", "Prodia Lab Semarang", false},
		{"Random Store", "Toko Sumber Rejeki", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValidHospital(tc.input)
			if got != tc.expected {
				t.Errorf("IsValidHospital(%q) = %v, expected %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestHaversineDistance(t *testing.T) {
	// Distance between RSUP Dr. Kariadi (-6.9934, 110.4087) and Simpang Lima (-6.9893, 110.4229)
	// Approx ~ 1.63 km
	dist := Haversine(-6.9934, 110.4087, -6.9893, 110.4229)
	if dist < 1.0 || dist > 2.5 {
		t.Errorf("expected distance between 1.0 and 2.5 km, got %f", dist)
	}

	// Identical coordinates should be 0
	zeroDist := Haversine(-6.9934, 110.4087, -6.9934, 110.4087)
	if math.Abs(zeroDist) > 0.0001 {
		t.Errorf("expected 0 distance, got %f", zeroDist)
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"RSUP Dr. Kariadi", "rsup-dr-kariadi"},
		{"SMC RS Telogorejo", "smc-rs-telogorejo"},
		{"RSUD Tugurejo Semarang!", "rsud-tugurejo-semarang"},
	}

	for _, tc := range tests {
		got := GenerateID(tc.input)
		if got != tc.expected {
			t.Errorf("GenerateID(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}
