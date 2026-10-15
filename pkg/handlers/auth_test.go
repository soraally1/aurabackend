package handlers

import (
	"strings"
	"testing"
)

func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}

func stringPtr(s string) *string {
	return &s
}

func TestValidateRegister(t *testing.T) {
	tests := []struct {
		name    string
		req     registerRequest
		wantErr string
	}{
		{
			name: "valid request",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "",
		},
		{
			name: "missing name",
			req: registerRequest{
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "name is required",
		},
		{
			name: "missing email",
			req: registerRequest{
				Name:         "Daffa",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "email is required",
		},
		{
			name: "invalid email",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "not-an-email",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "email is invalid",
		},
		{
			name: "short password",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "short",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "password must be at least 8 characters",
		},
		{
			name: "missing usia",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         nil,
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "usia is required",
		},
		{
			name: "invalid usia 0",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(0),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "usia must be between 1 and 150",
		},
		{
			name: "missing tinggi_badan",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  nil,
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "tinggi_badan is required",
		},
		{
			name: "invalid tinggi_badan <= 0",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(-5),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "tinggi_badan must be between 1 and 300 cm",
		},
		{
			name: "missing berat_badan",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   nil,
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "berat_badan is required",
		},
		{
			name: "invalid berat_badan <= 0",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(0),
				WaktuDiJalan: floatPtr(2),
			},
			wantErr: "berat_badan must be between 1 and 500 kg",
		},
		{
			name: "missing waktu_di_jalan",
			req: registerRequest{
				Name:        "Daffa",
				Email:       "daffa@example.com",
				Password:    "password123",
				Usia:        intPtr(25),
				TinggiBadan: floatPtr(170),
				BeratBadan:  floatPtr(65),
			},
			wantErr: "waktu_di_jalan is required",
		},
		{
			name: "valid waktu_di_jalan 0 (e.g. WFH)",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(0),
			},
			wantErr: "",
		},
		{
			name: "invalid waktu_di_jalan > 24",
			req: registerRequest{
				Name:         "Daffa",
				Email:        "daffa@example.com",
				Password:     "password123",
				Usia:         intPtr(25),
				TinggiBadan:  floatPtr(170),
				BeratBadan:   floatPtr(65),
				WaktuDiJalan: floatPtr(25),
			},
			wantErr: "waktu_di_jalan must be between 0 and 24 hours",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateRegister(tt.req)
			if got != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, got)
			}
		})
	}
}

func TestValidateUpdateProfile(t *testing.T) {
	tests := []struct {
		name    string
		req     updateProfileRequest
		wantErr string
	}{
		{
			name:    "empty request",
			req:     updateProfileRequest{},
			wantErr: "at least one field must be provided",
		},
		{
			name: "valid partial update usia",
			req: updateProfileRequest{
				Usia: intPtr(26),
			},
			wantErr: "",
		},
		{
			name: "valid partial update berat_badan",
			req: updateProfileRequest{
				BeratBadan: floatPtr(68.5),
			},
			wantErr: "",
		},
		{
			name: "valid partial update tinggi_badan and waktu_di_jalan",
			req: updateProfileRequest{
				TinggiBadan:  floatPtr(175.5),
				WaktuDiJalan: floatPtr(1.5),
			},
			wantErr: "",
		},
		{
			name: "empty name",
			req: updateProfileRequest{
				Name: stringPtr("   "),
			},
			wantErr: "name cannot be empty",
		},
		{
			name: "invalid usia",
			req: updateProfileRequest{
				Usia: intPtr(-1),
			},
			wantErr: "usia must be between 1 and 150",
		},
		{
			name: "invalid tinggi_badan",
			req: updateProfileRequest{
				TinggiBadan: floatPtr(350),
			},
			wantErr: "tinggi_badan must be between 1 and 300 cm",
		},
		{
			name: "invalid berat_badan",
			req: updateProfileRequest{
				BeratBadan: floatPtr(0),
			},
			wantErr: "berat_badan must be between 1 and 500 kg",
		},
		{
			name: "invalid waktu_di_jalan",
			req: updateProfileRequest{
				WaktuDiJalan: floatPtr(-1),
			},
			wantErr: "waktu_di_jalan must be between 0 and 24 hours",
		},
		{
			name: "valid name update",
			req: updateProfileRequest{
				Name: stringPtr("Daffa Updated"),
			},
			wantErr: "",
		},
		{
			name: "name exceeds max length",
			req: updateProfileRequest{
				Name: stringPtr(strings.Repeat("a", 101)),
			},
			wantErr: "name must be at most 100 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateUpdateProfile(tt.req)
			if got != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, got)
			}
		})
	}
}

func TestValidateOptionalProfile(t *testing.T) {
	tests := []struct {
		name    string
		reqName *string
		usia    *int
		tinggi  *float64
		berat   *float64
		waktu   *float64
		wantErr string
	}{
		{
			name:    "all nil fields (valid for google login without extra fields)",
			wantErr: "",
		},
		{
			name:    "valid all fields",
			reqName: stringPtr("Google User"),
			usia:    intPtr(28),
			tinggi:  floatPtr(172),
			berat:   floatPtr(68),
			waktu:   floatPtr(2.5),
			wantErr: "",
		},
		{
			name:    "empty name",
			reqName: stringPtr("   "),
			wantErr: "name cannot be empty",
		},
		{
			name:    "name too long",
			reqName: stringPtr(strings.Repeat("x", 101)),
			wantErr: "name must be at most 100 characters",
		},
		{
			name:    "invalid usia negative",
			usia:    intPtr(-10),
			wantErr: "usia must be between 1 and 150",
		},
		{
			name:    "invalid tinggi zero",
			tinggi:  floatPtr(0),
			wantErr: "tinggi_badan must be between 1 and 300 cm",
		},
		{
			name:    "invalid berat zero",
			berat:   floatPtr(0),
			wantErr: "berat_badan must be between 1 and 500 kg",
		},
		{
			name:    "invalid waktu negative",
			waktu:   floatPtr(-0.5),
			wantErr: "waktu_di_jalan must be between 0 and 24 hours",
		},
		{
			name:    "invalid waktu over 24",
			waktu:   floatPtr(25),
			wantErr: "waktu_di_jalan must be between 0 and 24 hours",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateOptionalProfile(tt.reqName, tt.usia, tt.tinggi, tt.berat, tt.waktu)
			if got != tt.wantErr {
				t.Fatalf("expected %q, got %q", tt.wantErr, got)
			}
		})
	}
}
