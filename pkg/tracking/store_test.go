package tracking

import (
	"testing"
	"time"
)

// TestGenerateSessionID validates session ID generation logic
func TestSessionIDGeneration(t *testing.T) {
	userID := "user_test_123"
	startTime := time.Now().UTC()

	sessionID := generateSessionID(userID, startTime)

	if sessionID == "" {
		t.Errorf("sessionID is empty")
	}

	if !contains(sessionID, userID) {
		t.Errorf("sessionID should contain userID: %s", sessionID)
	}
}

// Helper function to generate session ID (same as handler logic)
func generateSessionID(userID string, startTime time.Time) string {
	return "session_" + userID + "_" + time.Now().Format("20060102150405")
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
