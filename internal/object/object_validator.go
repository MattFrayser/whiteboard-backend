package object

import (
	"encoding/json"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

// Validator: validation and sanitization of drawing objects
type Validator struct {
	validate  *validator.Validate
	sanitizer *bluemonday.Policy
}

func NewValidator() *Validator {
	// removes all HTML/scripts
	policy := bluemonday.StrictPolicy()

	return &Validator{
		validate:  validator.New(validator.WithRequiredStructEnabled()),
		sanitizer: policy,
	}
}

// ValidateAndSanitize: validates object data against its schemas, sanitizes string fields
func (v *Validator) ValidateAndSanitize(objType string, data map[string]interface{}) (map[string]interface{}, error) {
	// object type is in whitelist
	if !AllowedObjectTypes[objType] {
		return nil, fmt.Errorf("invalid object type: %s (allowed types: rect, circle, ellipse, line, path, text, image, polygon, arrow)", objType)
	}

	//  schema struct for this object type
	schema := GetSchemaForType(objType)
	if schema == nil {
		return nil, fmt.Errorf("no schema found for object type: %s", objType)
	}

	// Convert map[string]interface{} to typed struct
	if err := mapToStruct(data, schema); err != nil {
		return nil, fmt.Errorf("failed to parse object data: %w", err)
	}

	// Validate the struct 
	if err := v.validate.Struct(schema); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			return nil, formatValidationErrors(validationErrors)
		}
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Sanitize all string fields in original data map
	sanitizedData := v.sanitizeMap(data)

	return sanitizedData, nil
}

// mapToStruct: converts a map[string]interface{} to a typed struct using JSON marshaling
func mapToStruct(data map[string]interface{}, target interface{}) error {
	// Marshal map to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Unmarshal JSON to struct
	if err := json.Unmarshal(jsonData, target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

// sanitizeMap recursively sanitizes all string values in a map
func (validator *Validator) sanitizeMap(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range data {
		result[key] = validator.sanitizeValue(value)
	}

	return result
}

// sanitizeValue sanitizes a value based on its type
func (validator *Validator) sanitizeValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		// Sanitize string to remove any HTML/scripts
		return validator.sanitizer.Sanitize(v)
	case map[string]interface{}:
		// Recursively sanitize nested maps
		return validator.sanitizeMap(v)
	case []interface{}:
		// Sanitize array elements
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = validator.sanitizeValue(item)
		}
		return result
	default:
		// Return non-string values as-is (numbers, bools, etc.)
		return value
	}
}

// formatValidationErrors converts validator errors to a user-friendly error message
// Simplified to provide clear, actionable feedback without excessive detail
func formatValidationErrors(errors validator.ValidationErrors) error {
	var messages []string
	for _, err := range errors {
		messages = append(messages, formatSingleError(err))
	}
	return fmt.Errorf("validation failed: %s", messages[0]) // Return first error for simplicity
}

// formatSingleError formats a single validation error with common cases
func formatSingleError(err validator.FieldError) string {
	field := err.Field()
	tag := err.Tag()

	switch tag {
	case "required":
		return fmt.Sprintf("'%s' is required", field)
	case "min", "max":
		return fmt.Sprintf("'%s' value out of allowed range", field)
	case "url":
		return fmt.Sprintf("'%s' must be a valid URL", field)
	default:
		return fmt.Sprintf("'%s' is invalid", field)
	}
}
