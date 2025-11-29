package validate

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
}

// ValidateRequest validates a request body against a struct
func ValidateRequest(next http.HandlerFunc, target interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode JSON body
		if err := json.NewDecoder(r.Body).Decode(target); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}

		// Validate struct
		if err := validate.Struct(target); err != nil {
			validationErrors := make(map[string]string)
			if ve, ok := err.(validator.ValidationErrors); ok {
				for _, fe := range ve {
					validationErrors[fe.Field()] = getValidationError(fe)
				}
			}
			respondValidationError(w, validationErrors)
			return
		}

		// Store validated struct in context or pass to next handler
		// For simplicity, we'll re-encode and pass it through
		// In a real implementation, you might want to use context
		next.ServeHTTP(w, r)
	}
}

// ValidateQuery validates query parameters
func ValidateQuery(next http.HandlerFunc, target interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters into struct
		// This is a simplified version - you may need to implement
		// query parameter parsing based on your needs
		if err := validate.Struct(target); err != nil {
			validationErrors := make(map[string]string)
			if ve, ok := err.(validator.ValidationErrors); ok {
				for _, fe := range ve {
					validationErrors[fe.Field()] = getValidationError(fe)
				}
			}
			respondValidationError(w, validationErrors)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// getValidationError returns a human-readable validation error message
func getValidationError(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fe.Field() + " is required"
	case "email":
		return fe.Field() + " must be a valid email address"
	case "min":
		return fe.Field() + " must be at least " + fe.Param() + " characters"
	case "max":
		return fe.Field() + " must be at most " + fe.Param() + " characters"
	case "uuid":
		return fe.Field() + " must be a valid UUID"
	case "gte":
		return fe.Field() + " must be greater than or equal to " + fe.Param()
	case "lte":
		return fe.Field() + " must be less than or equal to " + fe.Param()
	case "oneof":
		return fe.Field() + " must be one of: " + fe.Param()
	default:
		return fe.Field() + " is invalid"
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func respondValidationError(w http.ResponseWriter, errors map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "Validation failed",
		"details": errors,
	})
}



