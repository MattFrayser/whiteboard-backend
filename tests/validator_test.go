package tests

import (
	"testing"

	"main/internal/object"
)

func TestValidator_ValidRectangle(t *testing.T) {
	validator := object.NewValidator()

	data := map[string]interface{}{
		"x1":    float64(0),
		"y1":    float64(0),
		"x2":    float64(100),
		"y2":    float64(100),
		"color": "#000000",
		"width": float64(2),
	}

	sanitized, err := validator.ValidateAndSanitize("rectangle", data)
	if err != nil {
		t.Fatalf("Valid rectangle should pass: %v", err)
	}

	if sanitized == nil {
		t.Error("Sanitized data should not be nil")
	}
}

func TestValidator_InvalidObjectType(t *testing.T) {
	validator := object.NewValidator()

	data := map[string]interface{}{
		"x": float64(0),
		"y": float64(0),
	}

	_, err := validator.ValidateAndSanitize("malicious-type", data)
	if err == nil {
		t.Fatal("Should reject invalid object type")
	}

	expectedError := "invalid object type: malicious-type (allowed types: rectangle, circle, line, path, text, stroke)"
	if err.Error() != expectedError {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidator_XSSAttempt(t *testing.T) {
	validator := object.NewValidator()

	// Try to inject script in text object
	data := map[string]interface{}{
		"x":    float64(0),
		"y":    float64(0),
		"text": "<script>alert('xss')</script>Hello",
	}

	sanitized, err := validator.ValidateAndSanitize("text", data)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Script tags should be stripped
	sanitizedText := sanitized["text"].(string)
	if sanitizedText == "<script>alert('xss')</script>Hello" {
		t.Error("XSS attempt was not sanitized")
	}

	if sanitizedText != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", sanitizedText)
	}
}

func TestValidator_MissingRequiredFields(t *testing.T) {
	validator := object.NewValidator()

	// Rectangle missing required fields
	data := map[string]interface{}{
		"x1": float64(0),
		// Missing y1, x2, y2, color, width
	}

	_, err := validator.ValidateAndSanitize("rectangle", data)
	if err == nil {
		t.Fatal("Should reject object with missing required fields")
	}
}

func TestValidator_NestedSanitization(t *testing.T) {
	validator := object.NewValidator()

	// Stroke with nested malicious data (need at least 2 points)
	data := map[string]interface{}{
		"points": []interface{}{
			map[string]interface{}{
				"x":    float64(0),
				"y":    float64(0),
				"evil": "<img src=x onerror=alert('xss')>",
			},
			map[string]interface{}{
				"x": float64(10),
				"y": float64(10),
			},
		},
		"color": "#000000",
		"width": float64(2),
	}

	sanitized, err := validator.ValidateAndSanitize("stroke", data)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Check nested sanitization occurred
	points := sanitized["points"].([]interface{})
	firstPoint := points[0].(map[string]interface{})

	if evilVal, exists := firstPoint["evil"]; exists {
		if evilVal.(string) == "<img src=x onerror=alert('xss')>" {
			t.Error("Nested XSS was not sanitized")
		}
	}
}

func TestValidator_HTMLInColor(t *testing.T) {
	validator := object.NewValidator()

	data := map[string]interface{}{
		"x1":    float64(0),
		"y1":    float64(0),
		"x2":    float64(100),
		"y2":    float64(100),
		"color": "<script>alert('xss')</script>#000000",
		"width": float64(2),
	}

	sanitized, err := validator.ValidateAndSanitize("rectangle", data)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Script should be stripped from color field
	sanitizedColor := sanitized["color"].(string)
	if sanitizedColor == "<script>alert('xss')</script>#000000" {
		t.Error("HTML in color field was not sanitized")
	}
}
